package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"okb-web/internal/config"
	"okb-web/internal/okb"

	"github.com/gin-gonic/gin"
)

// ensureDeckSkill 把 deck/critic 等 skill 模板放到 ~/.openkb/skills/，让 OpenKB 全局可见。
//
// 来源是 okb-web 二进制内 embed 的副本（已由 bootstrap 释放到 <CacheDir>/skills/）。
// 之前从 OpenKB 源码目录里 symlink，现在切到完全自包含。
func ensureDeckSkill() error {
	skills := []string{"openkb-deck-editorial", "openkb-html-critic", "openkb"}
	home := os.Getenv("HOME")
	dst := filepath.Join(home, ".openkb", "skills")
	src := filepath.Join(config.C.CacheDir, "skills")

	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("内置 skills 未释放（%s），bootstrap 是否成功？", src)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, name := range skills {
		linkPath := filepath.Join(dst, name)
		// 如果已存在且指向我们 cache 里同名 skill，则跳过；其他情况都覆盖
		// （旧版本可能 symlink 到 ~/public_html/OpenKB/skills/...，这里强制更新）
		if _, err := os.Lstat(linkPath); err == nil {
			if target, err := os.Readlink(linkPath); err == nil {
				if target == filepath.Join(src, name) {
					continue
				}
			}
			_ = os.RemoveAll(linkPath)
		}
		if err := os.Symlink(filepath.Join(src, name), linkPath); err != nil {
			// symlink 失败（如跨文件系统、Windows 无权限）—— 兜底直接拷
			if err2 := copyDir(filepath.Join(src, name), linkPath); err2 != nil {
				return fmt.Errorf("install skill %s: symlink=%v copy=%v", name, err, err2)
			}
		}
	}
	return nil
}

// copyDir 简单递归拷贝（symlink 失败时的兜底）。
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		sp := filepath.Join(src, e.Name())
		dp := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(sp, dp); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(sp)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dp, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// ========== Deck (HTML 幻灯片导出) ==========
//
// 调用 OpenKB 的 `openkb deck new <name> "<intent>" [--critique] [-y]`，
// 产物落在 <space>/output/decks/<name>/index.html。
// 因为 LLM 生成耗时较长（数十秒~分钟），走异步 task 系统。

type DeckCreateReq struct {
	Space    string `json:"space" binding:"required"`
	Name     string `json:"name" binding:"required"`     // kebab-case slug
	Intent   string `json:"intent" binding:"required"`   // 自然语言意图
	Critique bool   `json:"critique"`                    // 是否开启二次评审（更慢更优）
}

// CreateDeck 触发 deck 生成，立即返回 task_id，前端轮询 /api/task/:id。
func CreateDeck(c *gin.Context) {
	var req DeckCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space / name / intent"})
		return
	}
	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}
	if !isValidName(req.Name) {
		c.JSON(400, gin.H{"error": "deck name 只能是英文/数字/下划线/短横线"})
		return
	}

	taskID := okb.NewTask(req.Space, 1)
	okb.UpdateTask(taskID, "running", "正在编译 deck（调用 LLM 中）...", nil)

	// 兜底：确保 OpenKB 自带的 deck skill 在全局可见
	if err := ensureDeckSkill(); err != nil {
		okb.UpdateTask(taskID, "error", "无法准备 deck skill: "+err.Error(), nil)
		c.JSON(200, gin.H{"success": true, "task_id": taskID})
		return
	}

	go func() {
		// 心跳：每 5s 更新一次「已运行 Xs」，让前端轮询看到时间在走
		var stopHeartbeat int32
		startedAt := time.Now()
		go func() {
			t := time.NewTicker(5 * time.Second)
			defer t.Stop()
			for atomic.LoadInt32(&stopHeartbeat) == 0 {
				<-t.C
				if atomic.LoadInt32(&stopHeartbeat) != 0 {
					return
				}
				elapsed := int(time.Since(startedAt).Seconds())
				okb.UpdateTask(taskID, "running",
					fmt.Sprintf("正在编译 deck「%s」（已运行 %ds，预计 30~90s）...", req.Name, elapsed), nil)
			}
		}()

		args := []string{"deck", "new", req.Name, req.Intent, "-y"}
		if req.Critique {
			args = append(args, "--critique")
		}
		success, _, stderr := okb.Run(args, spaceDir)
		atomic.StoreInt32(&stopHeartbeat, 1)
		elapsed := int(time.Since(startedAt).Seconds())

		if success {
			okb.UpdateTask(taskID, "done",
				fmt.Sprintf("deck「%s」生成完成（耗时 %ds）", req.Name, elapsed), []string{req.Name})
			// output/ 在 .gitignore 里，commit 是 no-op；保险起见仍触发，便于审计
			okb.GitCommit(spaceDir, "deck: "+req.Name)
		} else {
			msg := stderr
			if len(msg) > 400 {
				msg = msg[:400] + "..."
			}
			okb.UpdateTask(taskID, "error",
				fmt.Sprintf("deck 生成失败（%ds 后）: %s", elapsed, msg), nil)
		}
	}()

	c.JSON(200, gin.H{"success": true, "task_id": taskID})
}

// DeckInfo 单个 deck 的元信息。
type DeckInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`     // index.html 大小
	Modified int64  `json:"modified"` // unix 时间戳
	HasFile  bool   `json:"has_file"` // index.html 是否存在
}

// ListDecks 列出某空间下已生成的 deck。
func ListDecks(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	decksRoot := filepath.Join(spaceDir, "output", "decks")
	entries, err := os.ReadDir(decksRoot)
	if err != nil {
		c.JSON(200, []DeckInfo{}) // 还没生成过
		return
	}

	result := make([]DeckInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// 忽略 -workspace 迭代历史目录，只展示最终 deck
		if strings.HasSuffix(name, "-workspace") {
			continue
		}
		idx := filepath.Join(decksRoot, name, "index.html")
		info := DeckInfo{Name: name}
		if st, err := os.Stat(idx); err == nil {
			info.HasFile = true
			info.Size = st.Size()
			info.Modified = st.ModTime().Unix()
		}
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Modified > result[j].Modified })
	c.JSON(200, result)
}

// DeleteDeck 删除一个 deck（含 -workspace 迭代历史）。
func DeleteDeck(c *gin.Context) {
	space := c.Param("space")
	name := c.Param("name")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	if !isValidName(name) {
		c.JSON(400, gin.H{"error": "无效的 deck name"})
		return
	}
	deckPath := filepath.Join(spaceDir, "output", "decks", name)
	wsPath := filepath.Join(spaceDir, "output", "decks", name+"-workspace")
	os.RemoveAll(deckPath)
	os.RemoveAll(wsPath)
	c.JSON(200, gin.H{"success": true})
}

// ServeDeck 直接把 index.html 吐回浏览器（前端用 <a target="_blank">）。
// 该接口仅暴露 output/decks/<name>/index.html 这一种文件，不做通用文件下载。
func ServeDeck(c *gin.Context) {
	space := c.Param("space")
	name := c.Param("name")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	if !isValidName(name) {
		c.JSON(400, gin.H{"error": "无效的 deck name"})
		return
	}
	idx := filepath.Join(spaceDir, "output", "decks", name, "index.html")
	if _, err := os.Stat(idx); err != nil {
		c.JSON(404, gin.H{"error": "deck 不存在或尚未生成完成"})
		return
	}
	// 在线浏览（inline）；如需下载，前端可加 ?download=1 走 attachment
	if c.Query("download") == "1" {
		c.FileAttachment(idx, name+".html")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(c.Writer, c.Request, idx)
}
