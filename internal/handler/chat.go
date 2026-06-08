package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"okb-web/internal/config"
	"okb-web/internal/okb"

	"github.com/gin-gonic/gin"
)

// ========== Chat（多轮，复用 OpenKB chat session 持久化） ==========
//
// 通过 scripts/chat_helper.py（uv run --project OpenKB python ...）调底层 Agent SDK，
// 不走 OpenKB CLI 的 TTY REPL。session 文件落在 <space>/.openkb/chats/<id>.json。
//
// 接口：
//   GET    /api/chat/sessions/:space            → 列会话
//   GET    /api/chat/session/:space/:sid        → 加载某会话完整消息
//   DELETE /api/chat/session/:space/:sid        → 删除会话
//   POST   /api/chat/send                       → 发送消息（异步 task，因 LLM 慢）
//                                                 body: {space, session_id?, message}
//                                                 返回: {task_id}

// runChatHelper spawns the python helper, feeds JSON via stdin, and parses JSON stdout.
//
// 用 bootstrap 装好的 OpenKB venv 的 python 直接跑释放后的 chat_helper.py，
// 不再走 `uv run --project`：省一次 uv 启动开销 + 不依赖 cwd。
func runChatHelper(spaceDir string, payload map[string]any) (map[string]any, error) {
	scriptPath := filepath.Join(config.C.CacheDir, "chat_helper.py")

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(config.C.OpenKBPython, scriptPath)
	cmd.Stdin = strings.NewReader(string(body))
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helper exec: %v; stderr=%s", err, errBuf.String())
	}

	// stdout 末行为 JSON（前面可能有 LiteLLM 的 warning）
	stdout := strings.TrimSpace(out.String())
	// 找最后一个 { 起始的行
	lines := strings.Split(stdout, "\n")
	var jsonLine string
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") {
			jsonLine = l
			break
		}
	}
	if jsonLine == "" {
		return nil, fmt.Errorf("no JSON in helper output: %s", stdout)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonLine), &result); err != nil {
		return nil, fmt.Errorf("parse helper JSON: %v; line=%s", err, jsonLine)
	}
	return result, nil
}

// ChatListSessions GET /api/chat/sessions/:space
// 直接读 <space>/.openkb/chats/*.json，不 spawn Python（避免 4s 启动开销）。
func ChatListSessions(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	chatsDir := filepath.Join(spaceDir, ".openkb", "chats")
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		c.JSON(200, gin.H{"ok": true, "sessions": []any{}})
		return
	}
	type SessionMeta struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		Title      string `json:"title"`
		TurnCount  int    `json:"turn_count"`
		UpdatedAt  string `json:"updated_at"`
	}
	sessions := make([]SessionMeta, 0)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(chatsDir, e.Name()))
		if err != nil {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		m := SessionMeta{
			ID:        getStr(raw, "id"),
			Model:     getStr(raw, "model"),
			Title:     getStr(raw, "title"),
			UpdatedAt: getStr(raw, "updated_at"),
		}
		// turn_count = len(user_turns)
		if turns, ok := raw["user_turns"].([]any); ok {
			m.TurnCount = len(turns)
		}
		if m.ID == "" {
			m.ID = strings.TrimSuffix(e.Name(), ".json")
		}
		sessions = append(sessions, m)
	}
	// 按 updated_at 降序
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })
	c.JSON(200, gin.H{"ok": true, "sessions": sessions})
}

// ChatLoadSession GET /api/chat/session/:space/:sid
// 直接读 JSON，用 user_turns + assistant_texts 配对（已是人话，不含 tool 噪音）。
func ChatLoadSession(c *gin.Context) {
	space := c.Param("space")
	sid := c.Param("sid")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	if !isSafeSeg(sid) {
		c.JSON(400, gin.H{"error": "非法 session id"})
		return
	}
	path := filepath.Join(spaceDir, ".openkb", "chats", sid+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		c.JSON(404, gin.H{"ok": false, "error": "session not found"})
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		c.JSON(500, gin.H{"ok": false, "error": "parse session: " + err.Error()})
		return
	}

	type Msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := make([]Msg, 0)
	userTurns, _ := raw["user_turns"].([]any)
	asstTexts, _ := raw["assistant_texts"].([]any)
	maxLen := len(userTurns)
	if len(asstTexts) > maxLen {
		maxLen = len(asstTexts)
	}
	for i := 0; i < maxLen; i++ {
		if i < len(userTurns) {
			if s, ok := userTurns[i].(string); ok {
				msgs = append(msgs, Msg{Role: "user", Content: s})
			}
		}
		if i < len(asstTexts) {
			if s, ok := asstTexts[i].(string); ok {
				msgs = append(msgs, Msg{Role: "assistant", Content: s})
			}
		}
	}

	c.JSON(200, gin.H{
		"ok":         true,
		"session_id": getStr(raw, "id"),
		"title":      getStr(raw, "title"),
		"messages":   msgs,
	})
}

// ChatDeleteSession DELETE /api/chat/session/:space/:sid
// 直接 os.Remove。
func ChatDeleteSession(c *gin.Context) {
	space := c.Param("space")
	sid := c.Param("sid")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	if !isSafeSeg(sid) {
		c.JSON(400, gin.H{"error": "非法 session id"})
		return
	}
	path := filepath.Join(spaceDir, ".openkb", "chats", sid+".json")
	if err := os.Remove(path); err != nil {
		c.JSON(404, gin.H{"ok": false, "error": "remove failed: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "session_id": sid})
}

func getStr(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// ChatSendReq POST /api/chat/send body
type ChatSendReq struct {
	Space     string `json:"space" binding:"required"`
	SessionID string `json:"session_id"` // 可选，空则新建
	Message   string `json:"message" binding:"required"`
}

// ChatSend 异步：返回 task_id，前端 pollTask 拿到 done 状态后从 task.Files[0] 取 session_id。
func ChatSend(c *gin.Context) {
	var req ChatSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space / message"})
		return
	}
	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}

	taskID := okb.NewTask(req.Space, 1)
	okb.UpdateTask(taskID, "running", "正在思考...", nil)

	go func() {
		var stop int32
		started := time.Now()
		go func() {
			t := time.NewTicker(3 * time.Second)
			defer t.Stop()
			for atomic.LoadInt32(&stop) == 0 {
				<-t.C
				if atomic.LoadInt32(&stop) != 0 {
					return
				}
				okb.UpdateTask(taskID, "running",
					fmt.Sprintf("思考中（已用 %ds）...", int(time.Since(started).Seconds())), nil)
			}
		}()

		payload := map[string]any{
			"action":  "send",
			"kb_dir":  spaceDir,
			"message": req.Message,
		}
		if req.SessionID != "" {
			payload["session_id"] = req.SessionID
		}
		res, err := runChatHelper(spaceDir, payload)
		atomic.StoreInt32(&stop, 1)

		if err != nil {
			okb.UpdateTask(taskID, "error", "调用失败: "+err.Error(), nil)
			return
		}
		ok, _ := res["ok"].(bool)
		if !ok {
			msg, _ := res["error"].(string)
			if len(msg) > 600 {
				msg = msg[:600] + "..."
			}
			okb.UpdateTask(taskID, "error", "推理失败: "+msg, nil)
			return
		}
		// 把 answer 编码进 task message（前端解析）；session_id 放进 Files[0]。
		// task.Files 是 []string 字段，借用它存 [session_id, answer]
		sid, _ := res["session_id"].(string)
		ans, _ := res["answer"].(string)
		okb.UpdateTask(taskID, "done", "OK", []string{sid, ans})
	}()

	c.JSON(200, gin.H{"success": true, "task_id": taskID})
}

// ChatStreamReq POST /api/chat/stream body
type ChatStreamReq struct {
	Space     string `json:"space" binding:"required"`
	SessionID string `json:"session_id"`
	Message   string `json:"message" binding:"required"`
	Lang      string `json:"lang"` // "zh-CN" | "en"，决定 follow-ups 语言；空缺为 zh-CN
}

// ChatStream SSE 流式：spawn helper（stream 模式），把 NDJSON 转 SSE 推前端。
// 浏览器原生 EventSource 只支持 GET，前端用 fetch + ReadableStream 解析 SSE。
//
// 流尾巴：助手 done 后，本 handler 会再调一次轻量 LLM 生成 3 条 follow-up
// 问题，作为额外的 `follow_ups` SSE event 推给前端。失败时静默不发，不影响主答。
func ChatStream(c *gin.Context) {
	var req ChatStreamReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "缺少 space / message"})
		return
	}
	spaceDir, ok := resolveSpace(c, req.Space)
	if !ok {
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 关闭 nginx/代理缓冲

	flusher, _ := c.Writer.(http.Flusher)
	writeEvent := func(data string) {
		c.Writer.Write([]byte("data: "))
		c.Writer.Write([]byte(data))
		c.Writer.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}

	payload := map[string]any{
		"action":  "stream",
		"kb_dir":  spaceDir,
		"message": req.Message,
	}
	if req.SessionID != "" {
		payload["session_id"] = req.SessionID
	}
	body, _ := json.Marshal(payload)

	cmd := exec.Command(config.C.OpenKBPython, "-u",
		filepath.Join(config.C.CacheDir, "chat_helper.py"))
	cmd.Stdin = strings.NewReader(string(body))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeEvent(`{"event":"error","error":"pipe stdout failed"}`)
		return
	}
	cmd.Stderr = nil // 丢 litellm 警告

	if err := cmd.Start(); err != nil {
		writeEvent(fmt.Sprintf(`{"event":"error","error":"start helper: %s"}`, err))
		return
	}

	// 客户端断连时 kill helper
	notify := c.Request.Context().Done()
	done := make(chan struct{})
	go func() {
		select {
		case <-notify:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		case <-done:
		}
	}()

	// 在边转发边累积 answer，供 follow-ups 用。
	var answerBuf strings.Builder
	var hasDone bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		// 解析事件类型，仅累积 answer 文本（delta + done 兜底）
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			switch ev["event"] {
			case "delta":
				if t, ok := ev["text"].(string); ok {
					answerBuf.WriteString(t)
				}
			case "done":
				hasDone = true
				// done 里带 answer 时用它兜底（防 delta 漏字）
				if a, ok := ev["answer"].(string); ok && len(a) > answerBuf.Len() {
					answerBuf.Reset()
					answerBuf.WriteString(a)
				}
			}
		}
		writeEvent(line)
	}
	close(done)
	_ = cmd.Wait()
	if err := scanner.Err(); err != nil && err != io.EOF {
		writeEvent(fmt.Sprintf(`{"event":"error","error":"scan: %s"}`, err))
		return
	}

	// 客户端断连时也别再追 follow-ups
	select {
	case <-notify:
		return
	default:
	}

	// 仅在 done 收到、有内容、用户没断连时尝试生成 follow-ups
	if !hasDone || answerBuf.Len() < 20 {
		return
	}
	lang := req.Lang
	if lang != "en" {
		lang = "zh-CN"
	}
	fups := generateFollowUps(c.Request.Context(), spaceDir, req.Message, answerBuf.String(), lang)
	if len(fups) == 0 {
		return
	}
	// 输出 follow_ups 事件
	enc, err := json.Marshal(map[string]any{
		"event":      "follow_ups",
		"follow_ups": fups,
	})
	if err == nil {
		writeEvent(string(enc))
	}
}
