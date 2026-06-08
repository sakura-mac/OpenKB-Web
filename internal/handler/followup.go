package handler

import (
	"context"
	"log"
	"strings"
)

// generateFollowUps 在一轮对话结束后，让 LLM 生成 3 条「跟进问题」。
//
// 实现走 chat_helper.py 的 follow_ups action（spawn Python，复用 LiteLLM）。
// 这样可以白嫖 OpenKB 已经搞定的厂商兼容矩阵 —— DeepSeek / OpenAI / Anthropic /
// Gemini / Azure / Bedrock / Ollama 等所有 LiteLLM 支持的 provider 都能用，
// 不必我们自己手写每家协议（OpenAI 兼容、Anthropic Messages、Gemini generateContent
// 三套互不兼容的 body schema）。
//
// 代价：每次 spawn ~1-2s Python 启动开销。但 follow-ups 是流式 done 之后异步
// 生成，对用户感知是「答案已经出来 → 1-2s 后多出 3 个跟进按钮」，可以接受。
//
// ctx 取消（前端断连）时，runChatHelper 内部 cmd.Run 会因父 ctx 关闭被 kill。
// 这里用 spaceDir 而不是只用 LLM_BASE_URL，是因为 chat_helper 需要按 KB 配置
// 路由（每个 space 的 .openkb/config.yaml 可以有不同 model）。
func generateFollowUps(ctx context.Context, spaceDir, userQ, answer, lang string) []string {
	if userQ == "" || answer == "" {
		return nil
	}
	if lang != "en" {
		lang = "zh-CN"
	}

	payload := map[string]any{
		"action": "follow_ups",
		"kb_dir": spaceDir,
		"user_q": userQ,
		"answer": answer,
		"lang":   lang,
	}
	res, err := runChatHelper(spaceDir, payload)
	if err != nil {
		log.Printf("[follow-ups] helper error: %v", err)
		return nil
	}
	if ok, _ := res["ok"].(bool); !ok {
		if msg, _ := res["error"].(string); msg != "" {
			if len(msg) > 300 {
				msg = msg[:300]
			}
			log.Printf("[follow-ups] helper not ok: %s", msg)
		}
		return nil
	}
	raw, _ := res["follow_ups"].([]any)
	if len(raw) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, x := range raw {
		s, _ := x.(string)
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= 3 {
			break
		}
	}
	return out
}
