package handler

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"okb-web/internal/config"
	"okb-web/internal/model"
	"okb-web/internal/okb"

	"github.com/gin-gonic/gin"
)

// resolveSpace 校验空间名合法（防路径遍历）并确认空间存在，返回空间目录。
// 校验失败时已写入 HTTP 响应，调用方应直接 return。
func resolveSpace(c *gin.Context, name string) (string, bool) {
	if !isValidName(name) {
		c.JSON(400, gin.H{"error": "无效的空间名"})
		return "", false
	}
	dir := filepath.Join(config.C.SpacesRoot, name)
	if _, err := os.Stat(dir); err != nil {
		c.JSON(404, gin.H{"error": "空间不存在"})
		return "", false
	}
	return dir, true
}

// isSafeSeg 校验单个路径段，禁止包含分隔符或 .. （防路径遍历）。
func isSafeSeg(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.ContainsAny(s, "/\\") || strings.Contains(s, "..") {
		return false
	}
	return true
}

var wikiCategories = map[string]bool{
	"summaries": true, "concepts": true, "entities": true, "sources": true,
	// explorations: OpenKB chat agent 在对话中临时综合多个 wiki 页面写的分析笔记
	// （比如对比表、专题深挖）。agent 存到 wiki/explorations/，需要前后端都认它，
	// 否则答案里指向 explorations/ 的链接点击会报 400 非法参数。
	"explorations": true,
}

// ========== 空间初始化状态管理 ==========

type spaceStatus struct {
	Status string `json:"status"` // initializing | ready | error
	Error  string `json:"error,omitempty"`
}

var (
	spaceStatuses = make(map[string]*spaceStatus)
	statusMu      sync.RWMutex
)

// ========== Spaces ==========

func ListSpaces(c *gin.Context) {
	root := config.C.SpacesRoot
	entries, err := os.ReadDir(root)
	if err != nil {
		c.JSON(200, []model.SpaceInfo{})
		return
	}

	spaces := make([]model.SpaceInfo, 0)
	for _, e := range entries {
		name := e.Name()
		spaceDir := filepath.Join(root, name)
		// 跟随 symlink 判断是否为目录
		info, err := os.Stat(spaceDir)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(spaceDir, ".openkb")); err != nil {
			continue
		}
		wikiDir := filepath.Join(spaceDir, "wiki")
		// 获取实际路径（解析 symlink）
		realPath, _ := filepath.EvalSymlinks(spaceDir)
		if realPath == "" {
			realPath = spaceDir
		}
		spaces = append(spaces, model.SpaceInfo{
			Name:     name,
			Path:     realPath,
			Kind:     "kb",
			Docs:     countFiles(filepath.Join(spaceDir, "raw")),
			Concepts: countMdFiles(filepath.Join(wikiDir, "concepts")),
		})
	}
	sort.Slice(spaces, func(i, j int) bool { return spaces[i].Name < spaces[j].Name })
	c.JSON(200, spaces)
}

func SpaceDetail(c *gin.Context) {
	name := c.Param("name")
	spaceDir, ok := resolveSpace(c, name)
	if !ok {
		return
	}

	wikiDir := filepath.Join(spaceDir, "wiki")
	rawDir := filepath.Join(spaceDir, "raw")

	// titles 映射 raw-slug-without-ext → 人类可读标题（URL 抓取时存的）。
	// 当前由 AddDoc URL 流程负责写入；其他文档没有 entry 时保留 slug 即可。
	rawTitles := loadURLTitles(spaceDir)
	titles := map[string]string{}
	for slug, e := range rawTitles {
		if e.Title != "" {
			titles[slug] = e.Title
		}
	}

	docs := make([]model.DocInfo, 0)
	if entries, err := os.ReadDir(rawDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, _ := e.Info()
			var size int64
			var modified float64
			if info != nil {
				size = info.Size()
				modified = float64(info.ModTime().Unix())
			}
			fname := e.Name()
			slug := strings.TrimSuffix(fname, filepath.Ext(fname))
			d := model.DocInfo{Name: fname, Size: size, Modified: modified}
			if t, ok := rawTitles[slug]; ok {
				d.DisplayName = t.Title
				d.SourceURL = t.URL
			}
			docs = append(docs, d)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })

	c.JSON(200, model.SpaceDetail{
		Name:      name,
		Path:      spaceDir,
		Docs:      docs,
		Summaries: listMdNames(filepath.Join(wikiDir, "summaries")),
		Concepts:  listMdNames(filepath.Join(wikiDir, "concepts")),
		Entities:  listMdNames(filepath.Join(wikiDir, "entities")),
		// chat agent 写的对比/专题笔记；目录可能不存在（旧 KB 没用过），listMdNames 返回 [] 即可
		Explorations: listMdNames(filepath.Join(wikiDir, "explorations")),
		Titles:       titles,
	})
}

func WikiPage(c *gin.Context) {
	space := c.Param("space")
	category := c.Param("category")
	page := c.Param("page")

	if !isValidName(space) || !wikiCategories[category] || !isSafeSeg(page) {
		c.JSON(400, gin.H{"error": "非法参数"})
		return
	}
	pagePath := filepath.Join(config.C.SpacesRoot, space, "wiki", category, page+".md")

	content, err := os.ReadFile(pagePath)
	if err != nil {
		c.JSON(404, gin.H{"error": "页面不存在"})
		return
	}
	c.JSON(200, gin.H{"content": string(content)})
}

// ========== Create / Delete ==========

func CreateSpace(c *gin.Context) {
	var req model.CreateSpaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "名称只能包含英文/数字/下划线/短横线"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if !isValidName(name) {
		c.JSON(400, gin.H{"error": "名称只能包含英文/数字/下划线/短横线"})
		return
	}

	// 空间目录始终在 SpacesRoot 下创建
	spaceDir := filepath.Join(config.C.SpacesRoot, name)
	customPath := strings.TrimSpace(req.Path)
	if customPath != "" {
		// 展开 ~。Windows 上 $HOME 可能为空，必须用 os.UserHomeDir()
		// （Windows 上它读 USERPROFILE，跨平台兼容）。
		if strings.HasPrefix(customPath, "~/") || strings.HasPrefix(customPath, `~\`) {
			if home, err := os.UserHomeDir(); err == nil {
				customPath = filepath.Join(home, customPath[2:])
			}
		}
	}

	if _, err := os.Stat(spaceDir); err == nil {
		// 如果目录存在且有 .openkb，说明是完整的空间，拒绝重复创建
		if _, err2 := os.Stat(filepath.Join(spaceDir, ".openkb")); err2 == nil {
			c.JSON(409, gin.H{"error": "空间已存在"})
			return
		}
		// 目录存在但没有 .openkb（初始化失败残留），清理后重新创建
		os.RemoveAll(spaceDir)
	}

	// 创建目录 + 写 .env（同步，很快）。失败要让用户知道——
	// 之前 mkdir/writefile 错误被吃掉，CreateSpace 看起来"成功"但 init 一定挂。
	if err := os.MkdirAll(spaceDir, 0o755); err != nil {
		log.Printf("❌ CreateSpace MkdirAll [%s] failed: %v", spaceDir, err)
		c.JSON(500, gin.H{"error": fmt.Sprintf("创建目录失败：%v（路径：%s）", err, spaceDir)})
		return
	}
	envContent := fmt.Sprintf("LLM_API_KEY=%s\nLLM_BASE_URL=%s\n", config.C.LLMApiKey, config.C.LLMBaseURL)
	if err := os.WriteFile(filepath.Join(spaceDir, ".env"), []byte(envContent), 0o644); err != nil {
		log.Printf("❌ CreateSpace WriteFile .env [%s] failed: %v", spaceDir, err)
		c.JSON(500, gin.H{"error": fmt.Sprintf("写入 .env 失败：%v", err)})
		return
	}

	// 标记状态为初始化中
	statusMu.Lock()
	spaceStatuses[name] = &spaceStatus{Status: "initializing"}
	statusMu.Unlock()

	// 立即返回，后台异步执行 init
	c.JSON(200, gin.H{"success": true, "name": name, "status": "initializing"})

	// 后台 goroutine 执行 openkb init
	go func() {
		log.Printf("🆕 CreateSpace [%s] → openkb init in %s", name, spaceDir)
		success, stdout, stderr := okb.RunWithStdin(
			[]string{"init", "-m", config.C.LLMModel, "-l", config.C.LLMLanguage},
			spaceDir, "\n",
		)
		if !success {
			log.Printf("❌ openkb init [%s] failed:\n--- stdout ---\n%s\n--- stderr ---\n%s",
				name, stdout, stderr)
		}

		statusMu.Lock()
		defer statusMu.Unlock()

		if success {
			// 兜底：确保 config.yaml 存在
			configPath := filepath.Join(spaceDir, ".openkb", "config.yaml")
			if _, err := os.Stat(configPath); err != nil {
				os.MkdirAll(filepath.Join(spaceDir, ".openkb"), 0755)
				configContent := fmt.Sprintf("language: %s\nmodel: %s\npageindex_threshold: 20\n",
					config.C.LLMLanguage, config.C.LLMModel)
				os.WriteFile(configPath, []byte(configContent), 0644)
			}
			okb.GitInit(spaceDir)
			okb.GitCommit(spaceDir, "init: 初始化知识库")
			// 如果用户指定了自定义路径，创建符号链接指向源目录方便访问
			if customPath != "" {
				linkPath := filepath.Join(spaceDir, "source")
				os.Symlink(customPath, linkPath)
			}
			spaceStatuses[name] = &spaceStatus{Status: "ready"}
			// .openkb 已经创建好，挂上 raw/ watcher：用户后续手动放文件到 raw/ 也能自动编译
			ensureWatch(spaceDir)
		} else {
			spaceStatuses[name] = &spaceStatus{Status: "error", Error: stderr}
			os.RemoveAll(spaceDir)
		}

		// 5 分钟后清理状态记录
		go func() {
			time.Sleep(5 * time.Minute)
			statusMu.Lock()
			delete(spaceStatuses, name)
			statusMu.Unlock()
		}()
	}()
}

// ========== Space Status (轮询初始化进度) ==========

func SpaceStatus(c *gin.Context) {
	name := c.Param("name")

	statusMu.RLock()
	st, exists := spaceStatuses[name]
	statusMu.RUnlock()

	if !exists {
		// 没有状态记录，检查空间是否已存在且有 .openkb
		spaceDir := filepath.Join(config.C.SpacesRoot, name)
		if _, err := os.Stat(filepath.Join(spaceDir, ".openkb", "config.yaml")); err == nil {
			c.JSON(200, gin.H{"status": "ready"})
		} else if _, err := os.Stat(spaceDir); err == nil {
			c.JSON(200, gin.H{"status": "initializing"})
		} else {
			c.JSON(404, gin.H{"status": "not_found"})
		}
		return
	}

	c.JSON(200, gin.H{"status": st.Status, "error": st.Error})
}

func DeleteSpace(c *gin.Context) {
	var req model.DeleteSpaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "无效请求"})
		return
	}
	name := strings.TrimSpace(req.Name)
	spaceDir, ok := resolveSpace(c, name)
	if !ok {
		return
	}
	// 先卸下 watcher，避免删除目录时触发 fsnotify 事件 panic / 误触 openkb add
	removeWatch(spaceDir)
	// 如果是 symlink，先获取实际路径，删除实际目录，再删 symlink
	fi, err := os.Lstat(spaceDir)
	if err == nil && fi.Mode()&os.ModeSymlink != 0 {
		realPath, _ := filepath.EvalSymlinks(spaceDir)
		os.Remove(spaceDir) // 删除 symlink 本身
		if realPath != "" {
			os.RemoveAll(realPath) // 删除实际目录
		}
	} else {
		os.RemoveAll(spaceDir)
	}
	c.JSON(200, gin.H{"success": true})
}

// ========== Query ==========

func Query(c *gin.Context) {
	var req model.QueryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space 或 question"})
		return
	}

	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}

	success, stdout, stderr := okb.Run([]string{"query", req.Question}, spaceDir)
	if success {
		c.JSON(200, gin.H{"success": true, "answer": stdout})
	} else {
		c.JSON(200, gin.H{"success": false, "error": stderr})
	}
}

// ========== Documents ==========

func AddDoc(c *gin.Context) {
	var req model.AddDocReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space 或 path"})
		return
	}

	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}

	docPath := req.Path
	// URL 路径直接透传给 OpenKB（它内部支持 `openkb add <URL>` 抓网页/PDF）。
	// URL 抓取耗时长（30~120s），走异步 task 系统：返回 task_id 让前端轮询。
	isURL := strings.HasPrefix(docPath, "http://") || strings.HasPrefix(docPath, "https://")
	if isURL {
		taskID := okb.NewTask(req.Space, 1)

		// 先并发抓 <title>，结果写入 channel，OpenKB 跑完后用它给 raw 文件附人类可读标题。
		// 抓不到也无所谓，前端就退化为显示 slug。
		titleCh := make(chan string, 1)
		go func() {
			t := fetchPageTitle(c.Request.Context(), docPath)
			titleCh <- t
		}()

		// 进度文案优先用 title；没拿到就用 shortURL。
		initLabel := shortURL(docPath)
		okb.UpdateTask(taskID, "running",
			fmt.Sprintf("正在抓取 %s（OpenKB 调 LLM 编译中）...", initLabel), nil)

		go func() {
			spaceLock(req.Space).Lock()
			defer spaceLock(req.Space).Unlock()

			// snapshot raw 目录，用于跑完后识别"新增文件"。
			rawDir := filepath.Join(spaceDir, "raw")
			before := snapshotRawSlugs(rawDir)

			// 心跳：每 5s 更新一次「已运行 Xs」让前端看到进度
			var stop int32
			started := time.Now()
			// 拿到 title 后用它替换进度文案里的 URL
			label := initLabel
			go func() {
				select {
				case t := <-titleCh:
					if t != "" {
						label = t
					}
				case <-time.After(10 * time.Second):
					// 抓不到放弃
				}
			}()
			go func() {
				t := time.NewTicker(5 * time.Second)
				defer t.Stop()
				for atomic.LoadInt32(&stop) == 0 {
					<-t.C
					if atomic.LoadInt32(&stop) != 0 {
						return
					}
					elapsed := int(time.Since(started).Seconds())
					okb.UpdateTask(taskID, "running",
						fmt.Sprintf("正在抓取「%s」（已运行 %ds，预计 30~120s）...", label, elapsed), nil)
				}
			}()

			success, _, stderr := okb.Run([]string{"add", docPath}, spaceDir)
			atomic.StoreInt32(&stop, 1)
			elapsed := int(time.Since(started).Seconds())

			if success {
				// 找出新增的 slug，把 title/url 关联进去
				after := snapshotRawSlugs(rawDir)
				newSlugs := diffSlugs(before, after)
				// title 可能 goroutine 还没填回 label——这里再 select 一次（非阻塞）
				finalTitle := ""
				select {
				case t := <-titleCh:
					if t != "" {
						finalTitle = t
					}
				default:
					if label != initLabel {
						finalTitle = label
					}
				}
				for _, s := range newSlugs {
					saveURLTitle(spaceDir, s, finalTitle, docPath)
				}

				commitLabel := finalTitle
				if commitLabel == "" {
					commitLabel = shortURL(docPath)
				}
				if err := okb.GitCommit(spaceDir, fmt.Sprintf("add: %s", commitLabel)); err != nil {
					log.Printf("[add-url] git commit failed: %v", err)
				}
				doneLabel := commitLabel
				okb.UpdateTask(taskID, "done",
					fmt.Sprintf("「%s」抓取并编译完成（耗时 %ds）", doneLabel, elapsed), []string{docPath})
			} else {
				msg := stderr
				if len(msg) > 400 {
					msg = msg[:400] + "..."
				}
				okb.UpdateTask(taskID, "error",
					fmt.Sprintf("抓取失败（%ds 后）：%s", elapsed, msg), nil)
			}
		}()

		c.JSON(200, gin.H{"success": true, "task_id": taskID})
		return
	}

	// ---- 本地文件路径：同步处理，因为编译速度可控 ----
	if strings.HasPrefix(docPath, "~/") {
		home := os.Getenv("HOME")
		docPath = filepath.Join(home, docPath[2:])
	}
	if _, err := os.Stat(docPath); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("文件不存在: %s", docPath)})
		return
	}

	// 同步路径也走 spaceLock 保护，避免和后台 recompile / URL add 抢 git
	spaceLock(req.Space).Lock()
	defer spaceLock(req.Space).Unlock()

	// 屏蔽 watcher 对该文件的事件：openkb add 会把源文件拷到 raw/<basename>，
	// 触发 fsnotify Create/Write，没屏蔽就会让 watcher 又跑一遍 add（idempotent 但浪费）
	stopIgnore := ignoreRawPath(filepath.Join(spaceDir, "raw", filepath.Base(docPath)))
	defer stopIgnore()

	success, stdout, stderr := okb.Run([]string{"add", docPath}, spaceDir)
	errMsg := ""
	if !success {
		errMsg = stderr
	} else {
		// 与上传保持一致：成功后纳入 git 版本管理
		if err := okb.GitCommit(spaceDir, fmt.Sprintf("add: %s", filepath.Base(docPath))); err != nil {
			log.Printf("[add] git commit failed: %v", err)
		}
	}
	c.JSON(200, gin.H{"success": success, "output": stdout, "error": errMsg})
}

func UploadDoc(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}

	rawDir := filepath.Join(spaceDir, "raw")
	os.MkdirAll(rawDir, 0755)

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(400, gin.H{"error": "无法解析上传"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(400, gin.H{"error": "未收到文件"})
		return
	}

	// Step 1: 保存所有文件到 raw/ (瞬间完成)
	var savedFiles []string
	var savedPaths []string
	// 每个 dst 都先在 watcher 那挂 ignore，写文件触发的 Create/Write 事件被忽略；
	// 等编译流程跑完再 unignore（Step 3 末尾）。
	var unignores []func()
	for _, fh := range files {
		safeName := sanitizeFilename(fh.Filename)
		dst := filepath.Join(rawDir, safeName)
		unignores = append(unignores, ignoreRawPath(dst))

		src, err := fh.Open()
		if err != nil {
			continue
		}
		out, err := os.Create(dst)
		if err != nil {
			src.Close()
			continue
		}
		io.Copy(out, src)
		src.Close()
		out.Close()

		savedFiles = append(savedFiles, safeName)
		savedPaths = append(savedPaths, dst)
	}

	if len(savedFiles) == 0 {
		c.JSON(400, gin.H{"error": "文件保存失败"})
		return
	}

	// Step 2: 创建异步 task，立即返回
	taskID := okb.NewTask(space, len(savedFiles))

	// Step 3: 后台 goroutine 执行编译
	go func() {
		// 编译跑完再解除 watcher 屏蔽（Ignore 内部还会延迟 debounce+1s 才真的清，
		// 防止后台尚未消化的 fsnotify 事件提前激活）
		defer func() {
			for _, un := range unignores {
				un()
			}
		}()
		var warnings []string
		for i, dst := range savedPaths {
			okb.UpdateTask(taskID, "running",
				fmt.Sprintf("正在编译 (%d/%d): %s", i+1, len(savedPaths), savedFiles[i]), nil)
			success, _, stderr := okb.Run([]string{"add", dst}, spaceDir)
			if !success {
				warnings = append(warnings, fmt.Sprintf("%s: %s", savedFiles[i], stderr))
			}
		}
		// 编译完成，git commit
		msg := fmt.Sprintf("add: %s", strings.Join(savedFiles, ", "))
		okb.GitCommit(spaceDir, msg)

		if len(warnings) > 0 {
			okb.UpdateTask(taskID, "done",
				fmt.Sprintf("%d 个文件编译完成，%d 个有警告", len(savedFiles), len(warnings)), savedFiles)
		} else {
			okb.UpdateTask(taskID, "done",
				fmt.Sprintf("%d 个文件编译完成", len(savedFiles)), savedFiles)
		}
	}()

	c.JSON(200, gin.H{"success": true, "task_id": taskID, "files": savedFiles})
}

func RemoveDoc(c *gin.Context) {
	var req model.RemoveDocReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space 或 doc"})
		return
	}

	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}
	if !isSafeSeg(req.Doc) {
		c.JSON(400, gin.H{"error": "非法文档名"})
		return
	}

	// 删除 = openkb remove（更新 wiki 交叉引用，5~15s）+ recompile --all -y（重编全部，30~60s）
	// 总计 35~75s，必须走异步 task，否则前端体验是「点了没反应」。
	taskID := okb.NewTask(req.Space, 1)
	okb.UpdateTask(taskID, "running", fmt.Sprintf("正在删除「%s」并更新交叉引用...", req.Doc), nil)

	// 屏蔽 watcher 对该文件的 Remove 事件：openkb remove 会从 raw/ 删文件，
	// 不屏蔽就会让 watcher 又跑一遍 remove
	stopIgnore := ignoreRawPath(filepath.Join(spaceDir, "raw", req.Doc))

	go func() {
		defer stopIgnore()
		spaceLock(req.Space).Lock()
		defer spaceLock(req.Space).Unlock()

		// 心跳：每 3s 更新进度文案
		var stop int32
		started := time.Now()
		phase := "removing" // "removing" | "recompiling"
		go func() {
			t := time.NewTicker(3 * time.Second)
			defer t.Stop()
			for atomic.LoadInt32(&stop) == 0 {
				<-t.C
				if atomic.LoadInt32(&stop) != 0 {
					return
				}
				elapsed := int(time.Since(started).Seconds())
				var msg string
				if phase == "removing" {
					msg = fmt.Sprintf("正在删除「%s」并更新交叉引用（已运行 %ds）...", req.Doc, elapsed)
				} else {
					msg = fmt.Sprintf("文档已删，正在重新编译知识库（已运行 %ds，预计 30~60s）...", elapsed)
				}
				okb.UpdateTask(taskID, "running", msg, nil)
			}
		}()

		// 1) openkb remove
		success, _, stderr := okb.Run([]string{"remove", req.Doc, "--yes"}, spaceDir)
		if !success {
			atomic.StoreInt32(&stop, 1)
			msg := stderr
			if len(msg) > 400 {
				msg = msg[:400] + "..."
			}
			okb.UpdateTask(taskID, "error", "删除失败: "+msg, nil)
			return
		}

		// 2) 删除 raw 中对应的原始文件（任意扩展名）
		base := filepath.Join(spaceDir, "raw", req.Doc)
		os.Remove(base)
		if matches, _ := filepath.Glob(base + ".*"); matches != nil {
			for _, m := range matches {
				os.Remove(m)
			}
		}
		if err := okb.GitCommit(spaceDir, fmt.Sprintf("remove: %s", req.Doc)); err != nil {
			log.Printf("[remove] git commit failed: %v", err)
		}

		// 3) recompile --all -y 重新编译知识库
		phase = "recompiling"
		rcOK, _, rcStderr := okb.Run([]string{"recompile", "--all", "-y"}, spaceDir)
		atomic.StoreInt32(&stop, 1)
		elapsed := int(time.Since(started).Seconds())

		if !rcOK {
			msg := rcStderr
			if len(msg) > 400 {
				msg = msg[:400] + "..."
			}
			log.Printf("[remove→recompile] failed: %s", rcStderr)
			// 文档已删但 recompile 失败：报告半成功
			okb.UpdateTask(taskID, "error",
				fmt.Sprintf("文档已删，但重新编译失败（%ds 后）：%s", elapsed, msg), nil)
			return
		}
		if err := okb.GitCommit(spaceDir, "recompile: 删除文档后重新编译"); err != nil {
			log.Printf("[remove→recompile] git commit failed: %v", err)
		}
		okb.UpdateTask(taskID, "done",
			fmt.Sprintf("「%s」已删除并重新编译完成（耗时 %ds）", req.Doc, elapsed), []string{req.Doc})
	}()

	c.JSON(200, gin.H{"success": true, "task_id": taskID})
}

// ========== Frontend ==========

// ========== Task status ==========

func GetTaskStatus(c *gin.Context) {
	id := c.Param("id")
	task := okb.GetTask(id)
	if task == nil {
		c.JSON(404, gin.H{"error": "任务不存在"})
		return
	}
	c.JSON(200, task)
}

// ========== History / Revert / Recompile / Lint / Status ==========

func History(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	entries := okb.GitLog(spaceDir, 50)
	// Attach changed files to each entry
	type EntryWithFiles struct {
		okb.GitLogEntry
		Files []string `json:"files"`
	}
	var result []EntryWithFiles
	for _, e := range entries {
		files := okb.GitShowFiles(spaceDir, e.Hash)
		result = append(result, EntryWithFiles{GitLogEntry: e, Files: files})
	}
	if result == nil {
		result = []EntryWithFiles{}
	}
	c.JSON(200, result)
}

type RevertReq struct {
	Space string `json:"space"`
	Hash  string `json:"hash"`
}

func Revert(c *gin.Context) {
	var req RevertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "无效请求"})
		return
	}
	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}

	output, err := okb.GitRevert(spaceDir, req.Hash)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "error": output})
		return
	}

	// Recompile after revert to regenerate knowledge.
	// 单 space 锁内串行：避免和 add/remove 并发抢 .git/index.lock。
	go func() {
		spaceLock(req.Space).Lock()
		defer spaceLock(req.Space).Unlock()

		ok, _, stderr := okb.Run([]string{"recompile", "--all", "-y"}, spaceDir)
		if !ok {
			log.Printf("[revert→recompile] failed: %s", stderr)
			return
		}
		// revert + recompile 的产物一并 commit（GitRevert 已经动了 wiki/，这里再 commit 一次确保最终态完整）
		if err := okb.GitCommit(spaceDir, "recompile: revert 后重新编译"); err != nil {
			log.Printf("[revert→recompile] git commit failed: %v", err)
		}
	}()

	c.JSON(200, gin.H{"success": true, "output": output})
}

func Recompile(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}

	// 后台跑：单 space 互斥避免重复触发
	go func() {
		spaceLock(space).Lock()
		defer spaceLock(space).Unlock()

		ok, _, stderr := okb.Run([]string{"recompile", "--all", "-y"}, spaceDir)
		if !ok {
			log.Printf("[recompile] failed: %s", stderr)
			return
		}
		if err := okb.GitCommit(spaceDir, "recompile: 重新编译知识"); err != nil {
			log.Printf("[recompile] git commit failed: %v", err)
		}
	}()

	c.JSON(200, gin.H{"success": true, "message": "重新编译已开始（后台运行）"})
}

func Lint(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}

	success, stdout, stderr := okb.Run([]string{"lint"}, spaceDir)
	if success {
		c.JSON(200, gin.H{"success": true, "output": stdout})
	} else {
		c.JSON(200, gin.H{"success": false, "output": stdout, "error": stderr})
	}
}

func Status(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}

	success, stdout, _ := okb.Run([]string{"status"}, spaceDir)
	c.JSON(200, gin.H{"success": success, "output": stdout})
}

// ========== Graph (真实关系图谱) ==========

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// graphDirs 映射 wiki 子目录到前端节点类型。
var graphDirs = map[string]string{
	"summaries": "doc",
	"concepts":  "concept",
	"entities":  "entity",
	// chat agent 写的对比/专题笔记。前端 GraphView 当前用 doc/concept/entity 三类做颜色，
	// exploration 暂时映射为 concept（语义最接近：跨多文档抽象出来的话题）。
	// 如果以后想区分，前端 type 加一个 'exploration' 即可。
	"explorations": "concept",
}

type GraphNode struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Graph 遍历空间 wiki 目录，解析文档/概念/实体之间真实的 [[category/slug]] 链接，
// 返回真实节点与边（不再使用随机生成的伪关系）。
func Graph(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	wikiDir := filepath.Join(spaceDir, "wiki")

	// 1. 收集所有节点，id 统一为 "<category>/<slug>"，与 wikilink 写法一致
	nodes := map[string]GraphNode{}
	for dir, typ := range graphDirs {
		entries, err := os.ReadDir(filepath.Join(wikiDir, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".md")
			id := dir + "/" + slug
			nodes[id] = GraphNode{ID: id, Label: slug, Type: typ}
		}
	}

	// 2. 解析每个文件中的 [[...]] 链接，仅在两端节点都存在时连边（去重、去自环）
	edges := map[string]GraphEdge{}
	for dir := range graphDirs {
		dpath := filepath.Join(wikiDir, dir)
		entries, err := os.ReadDir(dpath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			srcID := dir + "/" + strings.TrimSuffix(e.Name(), ".md")
			content, err := os.ReadFile(filepath.Join(dpath, e.Name()))
			if err != nil {
				continue
			}
			for _, m := range wikilinkRe.FindAllStringSubmatch(string(content), -1) {
				target := strings.TrimSpace(m[1])
				// 兼容 Obsidian 风格 alias：[[category/slug|显示文本]] → 取 | 前的真实 target
				if pipe := strings.Index(target, "|"); pipe >= 0 {
					target = strings.TrimSpace(target[:pipe])
				}
				if target == srcID {
					continue
				}
				if _, exists := nodes[target]; !exists {
					continue // 跳过悬空链接
				}
				// 无序对去重：A->B 与 B->A 视为同一条边
				a, b := srcID, target
				if a > b {
					a, b = b, a
				}
				key := a + "||" + b
				if _, dup := edges[key]; !dup {
					edges[key] = GraphEdge{Source: srcID, Target: target}
				}
			}
		}
	}

	nodeList := make([]GraphNode, 0, len(nodes))
	for _, n := range nodes {
		nodeList = append(nodeList, n)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].ID < nodeList[j].ID })
	edgeList := make([]GraphEdge, 0, len(edges))
	for _, e := range edges {
		edgeList = append(edgeList, e)
	}

	c.JSON(200, gin.H{"nodes": nodeList, "edges": edgeList})
}

// ========== Browse (server file browser) ==========

type BrowseReq struct {
	Path string `json:"path"`
}

type BrowseItem struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func Browse(c *gin.Context) {
	var req BrowseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Path = ""
	}

	dir := req.Path
	if dir == "" {
		dir = os.Getenv("HOME")
	}
	// Expand ~
	if len(dir) > 1 && dir[0] == '~' && dir[1] == '/' {
		dir = filepath.Join(os.Getenv("HOME"), dir[2:])
	}

	info, err := os.Stat(dir)
	if err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("路径不存在: %s", dir)})
		return
	}
	if !info.IsDir() {
		c.JSON(400, gin.H{"error": "不是目录"})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(400, gin.H{"error": "无法读取目录"})
		return
	}

	items := make([]BrowseItem, 0)
	for _, e := range entries {
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		items = append(items, BrowseItem{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  size,
		})
	}

	c.JSON(200, gin.H{"path": dir, "items": items})
}

func ServeFrontend(webFS embed.FS) gin.HandlerFunc {
	sub, _ := fs.Sub(webFS, "web/dist")
	fileServer := http.FileServer(http.FS(sub))

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		// Try serving the exact file
		if f, err := webFS.Open("web/dist" + path); err == nil {
			f.Close()
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		// SPA fallback
		data, err := webFS.ReadFile("web/dist/index.html")
		if err != nil {
			c.String(404, "404")
			return
		}
		c.Data(200, "text/html; charset=utf-8", data)
	}
}

// ========== Helpers ==========

func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			count++
		}
	}
	return count
}

func countMdFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			count++
		}
	}
	return count
}

func listMdNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}
	// 显式初始化为非 nil 切片，避免目录存在但里面无 .md 时返回 nil →
	// JSON 序列化成 null → 前端 space.summaries.length 抛 "Cannot read properties of null"。
	names := make([]string, 0)
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".md") {
			names = append(names, strings.TrimSuffix(n, ".md"))
		}
	}
	sort.Strings(names)
	return names
}

func isValidName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("upload_%d.bin", time.Now().Unix())
	}
	return name
}

// shortURL 给 URL 截一段易读名字，用于 commit message 和 task 进度显示。
// "https://en.wikipedia.org/wiki/Jensen_Huang?ref=foo" → "wikipedia.org/wiki/Jensen_Huang"
func shortURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		// 无法解析就截断原串
		if len(raw) > 60 {
			return raw[:60] + "..."
		}
		return raw
	}
	host := strings.TrimPrefix(u.Host, "www.")
	path := u.Path
	if len(path) > 50 {
		path = path[:50] + "..."
	}
	short := host + path
	if short == "" {
		return raw
	}
	return short
}

// ========== Space 级互斥锁 ==========
//
// 同一 space 的 add / remove / recompile 必须串行，否则会出现两种问题：
//  1. .git/index.lock 抢锁失败导致 commit 丢失
//  2. OpenKB 内部的 wiki/ 写文件也可能被并发改坏
//
// 不同 space 之间互不影响。锁存在内存里，进程重启即清。
var (
	spaceLocksMu sync.Mutex
	spaceLocks   = make(map[string]*sync.Mutex)
)

func spaceLock(space string) *sync.Mutex {
	spaceLocksMu.Lock()
	defer spaceLocksMu.Unlock()
	if lk, ok := spaceLocks[space]; ok {
		return lk
	}
	lk := &sync.Mutex{}
	spaceLocks[space] = lk
	return lk
}

// snapshotRawSlugs 返回 raw 目录下所有文件的 slug（去扩展名）集合。
// 用于 URL add 前后对比，识别 OpenKB 新生成的文件以便回填标题。
func snapshotRawSlugs(rawDir string) map[string]bool {
	out := map[string]bool{}
	entries, err := os.ReadDir(rawDir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		out[slug] = true
	}
	return out
}

// diffSlugs 返回 after 中存在但 before 不存在的 slug 列表。
func diffSlugs(before, after map[string]bool) []string {
	var newSlugs []string
	for s := range after {
		if !before[s] {
			newSlugs = append(newSlugs, s)
		}
	}
	return newSlugs
}
