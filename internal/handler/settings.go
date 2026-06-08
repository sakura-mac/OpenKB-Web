package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"okb-web/internal/config"
)

// ========== 设置 (LLM API 配置 + spaces 路径) ==========
//
// 配置存到 <OKB_HOME>/config.json（包含 API key 等敏感字段）。
// 前端通过 GET /api/settings 拉当前配置（API key 已 mask），通过 POST /api/settings 保存。
//
// 设计原则：
//   - GET 永远不返回 API key 明文，只返回 mask 后的 sk-***xxxx
//   - POST 接受可选字段：未传或空字符串 = "保持不变"
//   - 特殊：LLMApiKey 字段如果传空字符串就保留原值；如果传 "__CLEAR__" 就清空
//   - POST 成功后立即热应用：chat handler 等下次读 config.Snapshot() 时拿到新值

// SettingsResponse 是 GET /api/settings 的响应。
// 注意：llm_api_key 是 mask 形式（sk-***abcd），不暴露明文。
type SettingsResponse struct {
	OKBHome     string `json:"okb_home"`
	SpacesRoot  string `json:"spaces_root"`
	OKBSpec     string `json:"okb_spec"`
	LLMApiKey   string `json:"llm_api_key"`   // mask
	LLMHasKey   bool   `json:"llm_has_key"`   // 真实是否设置了，给前端做"未配置"提示
	LLMBaseURL  string `json:"llm_base_url"`
	LLMModel    string `json:"llm_model"`
	LLMLanguage string `json:"llm_language"`
	// 状态：bootstrap 是否就绪
	OpenKBReady bool   `json:"openkb_ready"`
	OpenKBBin   string `json:"openkb_bin,omitempty"`
}

// GetSettings 返回当前配置（mask 敏感字段）。
func GetSettings(c *gin.Context) {
	cfg := config.Snapshot()
	c.JSON(http.StatusOK, SettingsResponse{
		OKBHome:     cfg.OKBHome,
		SpacesRoot:  cfg.SpacesRoot,
		OKBSpec:     cfg.OKBSpec,
		LLMApiKey:   maskKey(cfg.LLMApiKey),
		LLMHasKey:   cfg.LLMApiKey != "",
		LLMBaseURL:  cfg.LLMBaseURL,
		LLMModel:    cfg.LLMModel,
		LLMLanguage: cfg.LLMLanguage,
		OpenKBReady: cfg.OpenKBBin != "",
		OpenKBBin:   cfg.OpenKBBin,
	})
}

// SettingsPatch 是 POST /api/settings 的请求体。
// 所有字段可选；省略 / 空字符串 = "保持原值不变"。
// 唯一例外：LLMApiKey 如果传 "__CLEAR__" 就清空（明确删除）。
type SettingsPatch struct {
	SpacesRoot  string `json:"spaces_root"`
	OKBSpec     string `json:"okb_spec"`
	LLMApiKey   string `json:"llm_api_key"`
	LLMBaseURL  string `json:"llm_base_url"`
	LLMModel    string `json:"llm_model"`
	LLMLanguage string `json:"llm_language"`
}

// UpdateSettings 持久化用户提交的字段。
//
// patch 里空字符串 = "保持不变"；要显式清空某字段（如删除 API key），
// 前端传 "__CLEAR__" 字符串，由 config.Save 解析。
func UpdateSettings(c *gin.Context) {
	var patch SettingsPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: " + err.Error()})
		return
	}

	cfg := config.Config{
		SpacesRoot:  patch.SpacesRoot,
		OKBSpec:     patch.OKBSpec,
		LLMApiKey:   patch.LLMApiKey,
		LLMBaseURL:  patch.LLMBaseURL,
		LLMModel:    patch.LLMModel,
		LLMLanguage: patch.LLMLanguage,
	}
	if err := config.Save(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "save: " + err.Error()})
		return
	}

	// 返回最新（mask 后的）状态
	GetSettings(c)
}

// CheckSettings 测试当前 LLM 配置能否通：实际跑一次 mini chat completion。
// 用于设置页的"测试连接"按钮。
//
// 不依赖 OpenKB / Python，直接 HTTP 调 base_url + chat/completions（OpenAI 兼容协议）。
// 大多数厂商（OpenAI/DeepSeek/Anthropic via proxy/Mistral/...）都支持这个端点。
// 失败的话返回详细错误信息让用户排查。
func CheckSettings(c *gin.Context) {
	cfg := config.Snapshot()
	if cfg.LLMApiKey == "" {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "未配置 API key"})
		return
	}
	if cfg.LLMBaseURL == "" {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": "未配置 Base URL"})
		return
	}

	// model 名字可能带 provider 前缀（"deepseek/deepseek-chat"），调实际 API 时要剥掉
	model := cfg.LLMModel
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}

	endpoint := strings.TrimRight(cfg.LLMBaseURL, "/") + "/v1/chat/completions"
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 1,
		"stream":     false,
	}
	bodyJSON, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(bodyJSON)))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.LLMApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		c.JSON(http.StatusOK, gin.H{
			"ok":     true,
			"model":  model,
			"status": resp.StatusCode,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":     false,
		"status": resp.StatusCode,
		"error":  fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 400)),
	})
}

// maskKey 返回 sk-***xxxx 格式（保留前 3 + 后 4 个字符）。
// 用于 GET /api/settings 响应，不暴露明文给前端 / 不写到日志。
func maskKey(k string) string {
	if k == "" {
		return ""
	}
	n := len(k)
	if n <= 8 {
		return strings.Repeat("*", n)
	}
	return k[:3] + strings.Repeat("*", 6) + k[n-4:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
