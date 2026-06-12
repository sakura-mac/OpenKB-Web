package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"okb-web/internal/codegraph"
	"okb-web/internal/config"
)

// ============================================================
// 代码问答的多轮会话持久化（对齐文档问答的 chat session 体验）
//
// 存储：<OKB_HOME>/code-chats/<space>/<sid>.json
// 结构：{ id, space, title, updated_at, messages:[{role, content, tools?}] }
// ============================================================

type codeSessionMsg struct {
	Role      string              `json:"role"`
	Content   string              `json:"content"`
	Tools     []string            `json:"tools,omitempty"`
	Graph     []map[string]string `json:"graph,omitempty"`      // 该轮探索过的图谱节点（category/name）
	FollowUps []string            `json:"follow_ups,omitempty"` // 该轮生成的跟进问题
}

type codeSession struct {
	ID        string           `json:"id"`
	Space     string           `json:"space"`
	Title     string           `json:"title"`
	UpdatedAt string           `json:"updated_at"`
	Messages  []codeSessionMsg `json:"messages"`
}

func codeChatsDir(space string) string {
	return filepath.Join(config.C.OKBHome, "code-chats", space)
}

func codeSessionPath(space, sid string) string {
	return filepath.Join(codeChatsDir(space), sid+".json")
}

func loadCodeSession(space, sid string) (*codeSession, error) {
	data, err := os.ReadFile(codeSessionPath(space, sid))
	if err != nil {
		return nil, err
	}
	var s codeSession
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveCodeSessionFile(s *codeSession) error {
	dir := codeChatsDir(s.Space)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(codeSessionPath(s.Space, s.ID), data, 0o600)
}

// appendCodeTurn 把一轮（user + assistant）追加到 session，新建或更新。
// graph 是该轮 agent 探索过的图谱节点，随 assistant 消息一起持久化。
// 返回最终的 session_id 和 title。
func appendCodeTurn(space, sid, userMsg, asstMsg string, tools []string, graph []map[string]string) (string, string) {
	var s *codeSession
	if sid != "" && isSafeSeg(sid) {
		if loaded, err := loadCodeSession(space, sid); err == nil {
			s = loaded
		}
	}
	if s == nil {
		s = &codeSession{
			ID:    fmt.Sprintf("%d", time.Now().UnixNano()),
			Space: space,
		}
	}
	if s.Title == "" {
		s.Title = makeTitle(userMsg)
	}
	s.Messages = append(s.Messages,
		codeSessionMsg{Role: "user", Content: userMsg},
		codeSessionMsg{Role: "assistant", Content: asstMsg, Tools: tools, Graph: graph},
	)
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = saveCodeSessionFile(s)
	return s.ID, s.Title
}

// updateCodeTurnFollowUps 把跟进问题回填到最后一条 assistant 消息并存盘。
// follow-ups 是 done 之后异步生成的，所以单独二次更新已存的 session。
func updateCodeTurnFollowUps(space, sid string, fups []string) {
	if sid == "" || !isSafeSeg(sid) || len(fups) == 0 {
		return
	}
	s, err := loadCodeSession(space, sid)
	if err != nil {
		return
	}
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "assistant" {
			s.Messages[i].FollowUps = fups
			break
		}
	}
	_ = saveCodeSessionFile(s)
}

var titleTrim = regexp.MustCompile(`\s+`)

func makeTitle(q string) string {
	q = titleTrim.ReplaceAllString(strings.TrimSpace(q), " ")
	runes := []rune(q)
	if len(runes) > 24 {
		return string(runes[:24]) + "…"
	}
	return q
}

// ListCodeSessions GET /api/code/sessions/:space
func ListCodeSessions(c *gin.Context) {
	space := c.Param("space")
	if !isValidName(space) {
		c.JSON(200, gin.H{"ok": true, "sessions": []any{}})
		return
	}
	entries, err := os.ReadDir(codeChatsDir(space))
	if err != nil {
		c.JSON(200, gin.H{"ok": true, "sessions": []any{}})
		return
	}
	type meta struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		TurnCount int    `json:"turn_count"`
		UpdatedAt string `json:"updated_at"`
	}
	out := make([]meta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sid := strings.TrimSuffix(e.Name(), ".json")
		s, err := loadCodeSession(space, sid)
		if err != nil {
			continue
		}
		turns := 0
		for _, m := range s.Messages {
			if m.Role == "user" {
				turns++
			}
		}
		out = append(out, meta{ID: s.ID, Title: s.Title, TurnCount: turns, UpdatedAt: s.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	c.JSON(200, gin.H{"ok": true, "sessions": out})
}

// LoadCodeSession GET /api/code/session/:space/:sid
func LoadCodeSession(c *gin.Context) {
	space := c.Param("space")
	sid := c.Param("sid")
	if !isValidName(space) || !isSafeSeg(sid) {
		c.JSON(400, gin.H{"ok": false, "error": "非法参数"})
		return
	}
	s, err := loadCodeSession(space, sid)
	if err != nil {
		c.JSON(404, gin.H{"ok": false, "error": "session not found"})
		return
	}
	// 只返回 user/assistant 的人话消息（带 tools / graph / follow_ups）
	type msg struct {
		Role      string              `json:"role"`
		Content   string              `json:"content"`
		Tools     []string            `json:"tools,omitempty"`
		Graph     []map[string]string `json:"graph,omitempty"`
		FollowUps []string            `json:"follow_ups,omitempty"`
	}
	msgs := make([]msg, 0, len(s.Messages))
	for _, m := range s.Messages {
		msgs = append(msgs, msg{Role: m.Role, Content: m.Content, Tools: m.Tools, Graph: m.Graph, FollowUps: m.FollowUps})
	}
	c.JSON(200, gin.H{"ok": true, "session_id": s.ID, "title": s.Title, "messages": msgs})
}

// DeleteCodeSession DELETE /api/code/session/:space/:sid
func DeleteCodeSession(c *gin.Context) {
	space := c.Param("space")
	sid := c.Param("sid")
	if !isValidName(space) || !isSafeSeg(sid) {
		c.JSON(400, gin.H{"ok": false, "error": "非法参数"})
		return
	}
	if err := os.Remove(codeSessionPath(space, sid)); err != nil {
		c.JSON(404, gin.H{"ok": false, "error": "remove failed"})
		return
	}
	c.JSON(200, gin.H{"ok": true, "session_id": sid})
}

// CodeSuggestions GET /api/code/suggestions/:space?lang=zh-CN
// 让 LLM 基于代码库名生成 4 条用户可能问的代码问题。
func CodeSuggestions(c *gin.Context) {
	cs, err := readCodeSpace(c.Param("space"))
	if err != nil {
		c.JSON(200, gin.H{"suggestions": []string{}})
		return
	}
	lang := c.Query("lang")
	if lang != "en" {
		lang = "zh-CN"
	}

	cfg := config.Snapshot()
	if cfg.LLMApiKey == "" {
		c.JSON(200, gin.H{"suggestions": fallbackCodeSuggestions(lang)})
		return
	}
	baseURL := cfg.LLMBaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	model := cfg.LLMModel
	if model == "" {
		model = "deepseek/deepseek-chat"
	}

	// 拿几个高频符号作为线索（best-effort）
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	hint := ""
	if ok, out, _ := codegraph.Run(ctx, cs.Path, "files"); ok {
		hint = truncateText(strings.TrimSpace(out), 1500)
	}

	sys := "你帮用户想出探索一个陌生代码库时最值得问的问题。严格输出一个 JSON 数组，含 4 个字符串，不要 markdown、不要解释。每条问题具体、简短（≤22字），覆盖不同角度：整体架构 / 关键流程 / 某模块实现 / 调用关系。用中文。"
	if lang == "en" {
		sys = "Suggest the most useful questions to ask when exploring an unfamiliar codebase. Output strictly a JSON array of 4 strings, no markdown, no explanation. Each is specific and short (<=18 words), covering different angles: architecture / key flow / a module's implementation / call relations."
	}
	userMsg := fmt.Sprintf("代码库名称：%s\n项目路径：%s\n文件结构线索：\n%s", cs.Name, cs.Path, hint)

	content, err := llmComplete(ctx, model, baseURL, cfg.LLMApiKey, []map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": userMsg},
	}, 256)
	if err != nil {
		c.JSON(200, gin.H{"suggestions": fallbackCodeSuggestions(lang)})
		return
	}
	sug := parseJSONStringArray(content, 4)
	if len(sug) == 0 {
		sug = fallbackCodeSuggestions(lang)
	}
	c.JSON(200, gin.H{"suggestions": sug})
}

func fallbackCodeSuggestions(lang string) []string {
	if lang == "en" {
		return []string{
			"What is the overall architecture of this project?",
			"Where is the main entry point?",
			"How does a request flow through the code?",
			"List the core modules and their responsibilities.",
		}
	}
	return []string{
		"这个项目的整体架构是怎样的？",
		"程序的主入口在哪里？",
		"一个请求是如何在代码里流转的？",
		"列出核心模块及各自职责。",
	}
}

// generateCodeFollowUps 基于本轮 Q+A 让 LLM 生成 3 条代码向跟进问题（Go 直调，非流式）。
func generateCodeFollowUps(ctx context.Context, model, baseURL, apiKey, userQ, answer, lang string) []string {
	if userQ == "" || answer == "" {
		return nil
	}
	if lang != "en" {
		lang = "zh-CN"
	}
	if len(userQ) > 400 {
		userQ = userQ[:400]
	}
	if len(answer) > 1500 {
		answer = answer[:1500]
	}
	sys := "用户在分析一个代码库。读完问题和回答后，写出用户最可能继续问的 3 个跟进问题。严格输出一个 JSON 数组（3 个字符串），不要 markdown、不要解释。每条≤22字，具体、自然延续（如深入某实现、追问调用方、相关模块）。用中文。"
	if lang == "en" {
		sys = "The user is exploring a codebase. Write 3 follow-up questions they'd likely ask next. Output strictly a JSON array of 3 strings, no markdown. Each <=18 words, specific (deeper into an implementation, callers, related modules)."
	}
	content, err := llmComplete(ctx, model, baseURL, apiKey, []map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": "问题:\n" + userQ + "\n\n回答:\n" + answer},
	}, 256)
	if err != nil {
		return nil
	}
	return parseJSONStringArray(content, 3)
}

var jsonArrayRe = regexp.MustCompile(`\[\s*"[^"]*"(?:\s*,\s*"[^"]*")*\s*\]`)

// parseJSONStringArray 从 LLM 输出里抽取字符串数组（兼容裸 JSON / ```json``` 包裹 / 前后带解释）。
func parseJSONStringArray(content string, limit int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	var arr []string
	if json.Unmarshal([]byte(content), &arr) != nil {
		m := jsonArrayRe.FindString(content)
		if m == "" {
			return nil
		}
		if json.Unmarshal([]byte(m), &arr) != nil {
			return nil
		}
	}
	out := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, s := range arr {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}
