package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"okb-web/internal/codegraph"
)

// ============================================================
// Code Agent —— 让 LLM 通过工具多轮探索 CodeGraph 代码图谱
//
// 设计动机：CodeGraph 的 query/callers/... 只返回符号「位置元数据」
// （filePath / startLine / docstring / 调用热度），**不含实现代码**。
// 单次 query 喂给 LLM 等于让它「看着目录回答内容」——必然答不好。
//
// 正确做法（对齐文档问答的 agent 体验）：把 CodeGraph 各命令 + 读源码
// 封装成 LLM 的 function-calling 工具，让模型自主多轮：
//   search_symbol → 拿到 file+line → read_source 读真实代码 →
//   需要时 find_callers/find_callees/analyze_impact 补全调用链 → 综合回答。
//
// 每次工具调用都通过 SSE `tool` 事件实时推给前端（工具时间轴），
// 最终答案通过 `delta` 流式 + `done` 收尾。
// ============================================================

const (
	maxSourceLines = 260   // read_source 单次最多返回行数，防上下文爆炸
	maxToolOutput  = 12000 // 单个工具输出最大字符数
)

// codeAgentTools 返回 OpenAI function-calling 工具定义。
func codeAgentTools() []map[string]any {
	strParam := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	intParam := func(desc string) map[string]any {
		return map[string]any{"type": "integer", "description": desc}
	}
	fn := func(name, desc string, props map[string]any, required []string) map[string]any {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": desc,
				"parameters": map[string]any{
					"type":       "object",
					"properties": props,
					"required":   required,
				},
			},
		}
	}
	return []map[string]any{
		fn("search_symbol",
			"在代码库中按名称搜索符号（函数/类/方法/变量），返回匹配项的限定名、文件路径、起止行号和文档注释。这是探索代码的第一步——拿到符号位置后再用 read_source 读实现。",
			map[string]any{
				"query": strParam("要搜索的符号名或关键词，如 set_step_linkage_config"),
				"kind":  strParam("可选，按类型过滤：function / class / method"),
			},
			[]string{"query"}),
		fn("read_source",
			"读取指定文件的某段源码（按行号区间）。用它查看符号的真实实现，而不是只看元数据。",
			map[string]any{
				"path":       strParam("相对项目根的文件路径，如 app/controllers/api/entity/setting_controller.php"),
				"start_line": intParam("起始行号（含）"),
				"end_line":   intParam("结束行号（含）"),
			},
			[]string{"path", "start_line", "end_line"}),
		fn("find_callers",
			"查找调用某个符号的所有函数/方法（谁调用了它），返回调用方位置。用于理解一个函数被哪里使用。",
			map[string]any{"symbol": strParam("符号名")},
			[]string{"symbol"}),
		fn("find_callees",
			"查找某个符号内部调用的所有函数/方法（它调用了谁），用于理解一个函数的依赖。",
			map[string]any{"symbol": strParam("符号名")},
			[]string{"symbol"}),
		fn("analyze_impact",
			"分析修改某符号会影响到哪些代码（影响面分析），返回受影响的符号列表。",
			map[string]any{"symbol": strParam("符号名")},
			[]string{"symbol"}),
	}
}

const codeAgentSystemPrompt = `你是资深代码分析助手，可以调用工具探索一个已建好索引的代码库（CodeGraph 知识图谱）。

工作方式：
1. 先用 search_symbol 定位用户问到的符号，拿到文件路径和行号
2. 用 read_source 读取真实源码实现——不要只凭符号元数据（位置、调用热度）就下结论
3. 需要理解调用关系时用 find_callers / find_callees / analyze_impact
4. 信息足够后，用中文给出准确、有据可查的回答

规则：
- 必须基于真实读到的源码回答，引用时标注文件名和行号
- 一个问题通常需要 2~5 次工具调用，不要偷懒只查一次就回答
- 如果搜不到符号，尝试换关键词或更宽泛的搜索
- 最终回答用 Markdown，代码片段用代码块并标明语言`

// runCodeAgent 执行 agentic loop：流式调 LLM，解析 tool_calls 并执行，
// 直到模型给出最终文本答案。每个工具调用发 `tool` SSE 事件，过程文本发 `delta`。
//
// 不在内部发 done —— 返回最终 answer + 探索过的图谱节点（符号/文件），
// 由调用方（CodeStream）统一发 done（携带 session_id / title / graph）。
//
// messages: 已包含 system + 历史对话 + 当前 user 的完整消息列表。
// 返回：(最终答案, 工具时间轴, 图谱节点, error)
func runCodeAgent(ctx context.Context, model, baseURL, apiKey, workDir string, messages []map[string]any, writeEvent func(string)) (string, []string, []map[string]string, error) {
	emit := func(v any) {
		b, _ := json.Marshal(v)
		writeEvent(string(b))
	}

	base := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	chatURL := base + "/chat/completions"

	// DeepSeek 直连不认 litellm 的 provider/ 前缀
	actualModel := model
	if strings.Contains(baseURL, "deepseek.com") && strings.Contains(model, "/") {
		actualModel = strings.SplitN(model, "/", 2)[1]
	}

	tools := codeAgentTools()

	// 收集 agent 探索过的图谱节点（符号/文件），done 时给前端画 MiniGraph。
	var graph []map[string]string
	graphSeen := map[string]bool{}
	addGraph := func(category, name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := category + "/" + name
		if graphSeen[key] || len(graph) >= 12 {
			return
		}
		graphSeen[key] = true
		graph = append(graph, map[string]string{"category": category, "name": name})
	}

	var toolTrace []string

	// 不约束轮数：让 agent 一直探索直到自己给出最终答案（不再调工具）为止。
	// 安全兜底靠请求上下文 ctx —— 客户端断连 / 超时会让网络请求失败而退出。
	for {
		if err := ctx.Err(); err != nil {
			return "", toolTrace, graph, err
		}
		reqBody := map[string]any{
			"model":    actualModel,
			"stream":   true,
			"messages": messages,
			"tools":    tools,
		}
		payload, _ := json.Marshal(reqBody)

		httpReq, err := http.NewRequestWithContext(ctx, "POST", chatURL, strings.NewReader(string(payload)))
		if err != nil {
			return "", toolTrace, graph, fmt.Errorf("创建请求失败: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			return "", toolTrace, graph, fmt.Errorf("调用 LLM 失败: %w", err)
		}

		content, toolCalls, _, err := parseAgentStream(resp, emit)
		resp.Body.Close()
		if err != nil {
			return "", toolTrace, graph, err
		}

		// 没有工具调用 → content 是最终答案
		if len(toolCalls) == 0 {
			return content, toolTrace, graph, nil
		}

		// 有工具调用：把 assistant(tool_calls) 加进对话，逐个执行并回灌结果
		asstMsg := map[string]any{
			"role":       "assistant",
			"tool_calls": toolCallsToWire(toolCalls),
		}
		if content != "" {
			asstMsg["content"] = content
		}
		messages = append(messages, asstMsg)

		for _, tc := range toolCalls {
			emit(map[string]any{
				"event": "tool",
				"name":  tc.Name,
				"args":  truncateText(tc.Args, 120),
			})
			toolTrace = append(toolTrace, fmt.Sprintf("%s(%s)", tc.Name, truncateText(tc.Args, 80)))
			collectGraphNode(tc.Name, tc.Args, addGraph)
			result := executeCodeTool(ctx, workDir, tc.Name, tc.Args)
			messages = append(messages, map[string]any{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      result,
			})
		}
	}
}

// collectGraphNode 从一次工具调用的参数里抽取图谱节点（符号 / 文件）。
func collectGraphNode(name, argsJSON string, add func(category, name string)) {
	var args map[string]any
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	str := func(k string) string {
		if v, ok := args[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	switch name {
	case "search_symbol":
		add("symbol", str("query"))
	case "find_callers", "find_callees", "analyze_impact":
		add("symbol", str("symbol"))
	case "read_source":
		p := str("path")
		if p != "" {
			add("file", filepath.Base(p))
		}
	}
}

// llmComplete 非流式调一次 chat/completions，返回 assistant 文本。
// 用于生成 follow-ups / suggestions 这类一次性轻量任务。
func llmComplete(ctx context.Context, model, baseURL, apiKey string, messages []map[string]string, maxTokens int) (string, error) {
	base := strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	actualModel := model
	if strings.Contains(baseURL, "deepseek.com") && strings.Contains(model, "/") {
		actualModel = strings.SplitN(model, "/", 2)[1]
	}
	reqBody := map[string]any{
		"model":       actualModel,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": 0.7,
	}
	payload, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := readLimited(resp.Body, 1024)
		return "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty choices")
	}
	return out.Choices[0].Message.Content, nil
}

type agentToolCall struct {
	ID   string
	Name string
	Args string
}

// parseAgentStream 解析一轮 OpenAI 流式响应，累积 content（实时 emit delta）+ tool_calls 分片。
func parseAgentStream(resp *http.Response, emit func(any)) (content string, toolCalls []agentToolCall, finish string, err error) {
	if resp.StatusCode != 200 {
		b, _ := readLimited(resp.Body, 4096)
		return "", nil, "", fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	// tool_calls 按 index 累积（流式分片：name 一次给全，arguments 分多片）
	type acc struct {
		id   string
		name string
		args strings.Builder
	}
	accs := map[int]*acc{}
	var contentBuf strings.Builder

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.Delta.Content != "" {
			contentBuf.WriteString(ch.Delta.Content)
			emit(map[string]any{"event": "delta", "text": ch.Delta.Content})
		}
		for _, tc := range ch.Delta.ToolCalls {
			a := accs[tc.Index]
			if a == nil {
				a = &acc{}
				accs[tc.Index] = a
			}
			if tc.ID != "" {
				a.id = tc.ID
			}
			if tc.Function.Name != "" {
				a.name = tc.Function.Name
			}
			a.args.WriteString(tc.Function.Arguments)
		}
		if ch.FinishReason != "" {
			finish = ch.FinishReason
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, "", fmt.Errorf("读取流失败: %w", err)
	}

	// 按 index 顺序组装 tool_calls
	for i := 0; i < len(accs); i++ {
		a := accs[i]
		if a == nil || a.name == "" {
			continue
		}
		toolCalls = append(toolCalls, agentToolCall{ID: a.id, Name: a.name, Args: a.args.String()})
	}
	return contentBuf.String(), toolCalls, finish, nil
}

// toolCallsToWire 把内部 toolCall 转回 OpenAI message.tool_calls 线格式（回灌给下一轮）。
func toolCallsToWire(tcs []agentToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(tcs))
	for _, tc := range tcs {
		args := tc.Args
		if args == "" {
			args = "{}"
		}
		out = append(out, map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": args,
			},
		})
	}
	return out
}

// executeCodeTool 执行一个工具调用，返回喂回给 LLM 的文本结果。
func executeCodeTool(ctx context.Context, workDir, name, argsJSON string) string {
	var args map[string]any
	if argsJSON != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	getStr := func(k string) string {
		if v, ok := args[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	getInt := func(k string) int {
		switch v := args[k].(type) {
		case float64:
			return int(v)
		case string:
			var n int
			fmt.Sscanf(v, "%d", &n)
			return n
		}
		return 0
	}

	switch name {
	case "search_symbol":
		q := getStr("query")
		if q == "" {
			return "错误：缺少 query 参数"
		}
		cgArgs := []string{"query", q, "-j", "-l", "8"}
		if kind := getStr("kind"); kind != "" {
			cgArgs = append(cgArgs, "-k", kind)
		}
		ok, out, errOut := codegraph.Run(ctx, workDir, cgArgs...)
		return cgResult(ok, out, errOut)
	case "find_callers":
		s := getStr("symbol")
		if s == "" {
			return "错误：缺少 symbol 参数"
		}
		ok, out, errOut := codegraph.Run(ctx, workDir, "callers", s, "-j")
		return cgResult(ok, out, errOut)
	case "find_callees":
		s := getStr("symbol")
		if s == "" {
			return "错误：缺少 symbol 参数"
		}
		ok, out, errOut := codegraph.Run(ctx, workDir, "callees", s, "-j")
		return cgResult(ok, out, errOut)
	case "analyze_impact":
		s := getStr("symbol")
		if s == "" {
			return "错误：缺少 symbol 参数"
		}
		ok, out, errOut := codegraph.Run(ctx, workDir, "impact", s, "-j")
		return cgResult(ok, out, errOut)
	case "read_source":
		return readSource(workDir, getStr("path"), getInt("start_line"), getInt("end_line"))
	default:
		return "错误：未知工具 " + name
	}
}

func cgResult(ok bool, out, errOut string) string {
	out = strings.TrimSpace(out)
	if !ok {
		if errOut != "" {
			return "工具执行失败：" + truncateText(errOut, 600)
		}
		return "工具执行失败（无输出）"
	}
	if out == "" || out == "[]" {
		return "无结果。"
	}
	return truncateText(out, maxToolOutput)
}

// readSource 读 workDir 内某文件的 [start,end] 行，带行号前缀。
// 路径限制在 workDir 内，防止越权读任意文件。
func readSource(workDir, relPath string, start, end int) string {
	if relPath == "" {
		return "错误：缺少 path 参数"
	}
	clean := filepath.Clean(relPath)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return "错误：路径必须是项目内的相对路径"
	}
	full := filepath.Join(workDir, clean)
	// 二次校验：解析后仍在 workDir 内
	if rel, err := filepath.Rel(workDir, full); err != nil || strings.HasPrefix(rel, "..") {
		return "错误：路径越界"
	}
	f, err := os.Open(full)
	if err != nil {
		return "错误：无法打开文件 " + relPath + "（" + err.Error() + "）"
	}
	defer f.Close()

	if start < 1 {
		start = 1
	}
	if end < start {
		end = start + maxSourceLines - 1
	}
	if end-start+1 > maxSourceLines {
		end = start + maxSourceLines - 1
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "// %s 第 %d-%d 行\n", relPath, start, end)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	ln := 0
	got := false
	for sc.Scan() {
		ln++
		if ln < start {
			continue
		}
		if ln > end {
			break
		}
		got = true
		fmt.Fprintf(&sb, "%d\t%s\n", ln, sc.Text())
	}
	if !got {
		return fmt.Sprintf("错误：%s 在 %d-%d 行范围内没有内容（文件共 %d 行）", relPath, start, end, ln)
	}
	return truncateText(sb.String(), maxToolOutput)
}

func readLimited(r interface{ Read([]byte) (int, error) }, n int64) ([]byte, error) {
	buf := make([]byte, n)
	total := 0
	for int64(total) < n {
		m, err := r.Read(buf[total:])
		total += m
		if err != nil {
			break
		}
	}
	return buf[:total], nil
}
