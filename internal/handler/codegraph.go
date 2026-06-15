package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"okb-web/internal/codegraph"
	"okb-web/internal/config"
	"okb-web/internal/model"
	"okb-web/internal/okb"
)

type codeSpaceFile struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	CreatedAt int64  `json:"created_at"`
}

func codeSpacesRoot() string {
	return filepath.Join(config.C.OKBHome, "code-spaces")
}

func codeSpaceMetaPath(name string) string {
	return filepath.Join(codeSpacesRoot(), name+".json")
}

func readCodeSpace(name string) (codeSpaceFile, error) {
	var cs codeSpaceFile
	if !isValidName(name) {
		return cs, fmt.Errorf("无效的代码空间名")
	}
	data, err := os.ReadFile(codeSpaceMetaPath(name))
	if err != nil {
		return cs, err
	}
	if err := json.Unmarshal(data, &cs); err != nil {
		return cs, err
	}
	return cs, nil
}

func writeCodeSpace(cs codeSpaceFile) error {
	if err := os.MkdirAll(codeSpacesRoot(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(codeSpaceMetaPath(cs.Name), data, 0o600)
}

func codeSpaceInfo(cs codeSpaceFile) model.CodeSpaceInfo {
	idx := filepath.Join(cs.Path, ".codegraph")
	indexed := false
	if st, err := os.Stat(idx); err == nil && st.IsDir() {
		indexed = true
	}
	return model.CodeSpaceInfo{
		Name:       cs.Name,
		Path:       cs.Path,
		Kind:       "code",
		Indexed:    indexed,
		Files:      countCodeFiles(cs.Path),
		Codegraph:  idx,
		ModifiedAt: cs.CreatedAt,
	}
}

func ListCodeSpaces(c *gin.Context) {
	entries, err := os.ReadDir(codeSpacesRoot())
	if err != nil {
		c.JSON(http.StatusOK, []model.CodeSpaceInfo{})
		return
	}
	out := make([]model.CodeSpaceInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		cs, err := readCodeSpace(name)
		if err != nil {
			continue
		}
		out = append(out, codeSpaceInfo(cs))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	c.JSON(http.StatusOK, out)
}

func CodeSpaceDetail(c *gin.Context) {
	cs, err := readCodeSpace(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "代码空间不存在"})
		return
	}
	c.JSON(http.StatusOK, codeSpaceInfo(cs))
}

func CreateCodeSpace(c *gin.Context) {
	var req model.CreateCodeSpaceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 name/path"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if !isValidName(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称只能包含英文/数字/下划线/短横线"})
		return
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择代码目录"})
		return
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "代码目录不存在或不可访问"})
		return
	}
	realPath, _ := filepath.EvalSymlinks(path)
	if realPath == "" {
		realPath = path
	}
	cs := codeSpaceFile{Name: name, Path: realPath, CreatedAt: time.Now().Unix()}
	if err := writeCodeSpace(cs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入代码空间失败：" + err.Error()})
		return
	}

	taskID := okb.NewTask(name, 1)
	okb.UpdateTask(taskID, "running", "CodeGraph 索引中…", nil)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		success, stdout, stderr := codegraph.Run(ctx, realPath, "init", "-i")
		if success {
			okb.UpdateTask(taskID, "done", "CodeGraph 索引完成", []string{realPath})
		} else {
			msg := strings.TrimSpace(stderr)
			if msg == "" {
				msg = strings.TrimSpace(stdout)
			}
			okb.UpdateTask(taskID, "error", "CodeGraph 索引失败："+truncateText(msg, 800), nil)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"success": true, "task_id": taskID, "space": codeSpaceInfo(cs)})
}

func DeleteCodeSpace(c *gin.Context) {
	var req model.DeleteSpaceReq
	if err := c.ShouldBindJSON(&req); err != nil || !isValidName(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效请求"})
		return
	}
	_ = os.Remove(codeSpaceMetaPath(req.Name))
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func CodeStream(c *gin.Context) {
	var req model.CodeStreamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space"})
		return
	}
	cs, err := readCodeSpace(req.Space)
	if err != nil {
		c.JSON(404, gin.H{"error": "代码空间不存在"})
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(http.Flusher)
	writeEvent := func(data string) {
		c.Writer.Write([]byte("data: "))
		c.Writer.Write([]byte(data))
		c.Writer.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	writeObj := func(v any) {
		b, _ := json.Marshal(v)
		writeEvent(string(b))
	}

	cfg := config.Snapshot()
	apiKey := cfg.LLMApiKey
	baseURL := cfg.LLMBaseURL
	llmModel := cfg.LLMModel
	if apiKey == "" {
		writeObj(map[string]any{"event": "error", "error": "未配置 LLM API Key，请先在设置页填入"})
		return
	}
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	if llmModel == "" {
		llmModel = "deepseek-chat"
	}
	auxModel := cfg.LLMAuxModel // 辅助模型：fact-check / follow-ups 等轻量任务；为空则回退到主模型
	if auxModel == "" {
		auxModel = llmModel
	}

	// 组装多轮对话：system + 已存会话历史 + 当前 user。
	messages := []map[string]any{
		{"role": "system", "content": codeAgentSystemPrompt},
	}
	if req.SessionID != "" {
		if s, err := loadCodeSession(req.Space, req.SessionID); err == nil {
			for _, m := range s.Messages {
				if m.Role == "user" || m.Role == "assistant" {
					messages = append(messages, map[string]any{"role": m.Role, "content": m.Content})
				}
			}
		}
	}
	messages = append(messages, map[string]any{"role": "user", "content": req.Question})

	writeObj(map[string]any{"event": "start", "session_id": req.SessionID})

	answer, tools, graph, err := runBestOfNAgent(c.Request.Context(), llmModel, baseURL, apiKey, cs.Path, messages, writeEvent, 3, auxModel)
	if err != nil {
		select {
		case <-c.Request.Context().Done():
			return
		default:
			writeObj(map[string]any{"event": "error", "error": err.Error()})
			return
		}
	}

	// 保存会话（追加这一轮，含图谱节点）→ 拿到 session_id / title
	sid, title := appendCodeTurn(req.Space, req.SessionID, req.Question, answer, tools, graph)

	writeObj(map[string]any{
		"event":      "done",
		"answer":     answer,
		"session_id": sid,
		"title":      title,
		"graph":      graph,
	})

	// 客户端没断连 + 答案够长 → 异步生成 follow-ups
	select {
	case <-c.Request.Context().Done():
		return
	default:
	}
	if len([]rune(answer)) >= 20 {
		lang := req.Lang
		if lang != "en" {
			lang = "zh-CN"
		}
		fctx, fcancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		defer fcancel()
		fups := generateCodeFollowUps(fctx, auxModel, baseURL, apiKey, req.Question, answer, lang)
		if len(fups) > 0 {
			updateCodeTurnFollowUps(req.Space, sid, fups) // 回填持久化
			writeObj(map[string]any{"event": "follow_ups", "follow_ups": fups})
		} else {
			log.Printf("[follow_ups] 生成结果为空（LLM 失败或 JSON 解析失败），跳过")
		}
	} else {
		log.Printf("[follow_ups] 答案太短 (%d 字符)，跳过生成", len([]rune(answer)))
	}
}

func CodeQuery(c *gin.Context) {
	var req model.CodeQueryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "缺少 space/question"})
		return
	}
	cs, err := readCodeSpace(req.Space)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "代码空间不存在"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	success, stdout, stderr := codegraph.Run(ctx, cs.Path, "query", req.Question)
	if success {
		c.JSON(http.StatusOK, gin.H{"success": true, "answer": stdout})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": false, "error": firstNonEmpty(stderr, stdout)})
}

func SyncCodeSpace(c *gin.Context) {
	var req model.CodeSyncReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "缺少 space"})
		return
	}
	cs, err := readCodeSpace(req.Space)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "代码空间不存在"})
		return
	}
	taskID := okb.NewTask(req.Space, 1)
	okb.UpdateTask(taskID, "running", "CodeGraph 同步中…", nil)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		success, stdout, stderr := codegraph.Run(ctx, cs.Path, "sync")
		if success {
			okb.UpdateTask(taskID, "done", "CodeGraph 同步完成", []string{cs.Path})
		} else {
			okb.UpdateTask(taskID, "error", "CodeGraph 同步失败："+truncateText(firstNonEmpty(stderr, stdout), 800), nil)
		}
	}()
	c.JSON(http.StatusOK, gin.H{"success": true, "task_id": taskID})
}

func countCodeFiles(root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".codegraph", "node_modules", "vendor", "dist", "build", "target", ".venv", "Pods", ".next":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		count++
		return nil
	})
	return count
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func truncateText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
