// Package config 集中管理 okb-web 的运行时配置。
//
// 配置来源优先级（从高到低）：
//  1. 环境变量（OKB_HOME / SPACES_ROOT / LLM_API_KEY 等）
//  2. <OKB_HOME>/config.json（用户通过 Web 设置页保存的）
//  3. .env 文件（开发模式）
//  4. 内置默认值
//
// 路径布局（跨平台）：
//
//	OKB_HOME/                       ← 默认 os.UserConfigDir()/OKB
//	  ├── config.json               ← LLM 设置，前端"设置"页写入
//	  ├── spaces/                   ← 知识库（旧版本是 ./okb-spaces，仍兼容）
//	  ├── runtime/                  ← 首次启动下载的 Python + OpenKB venv
//	  └── cache/                    ← chat_helper.py / skills 释放点
//
// 平台 default OKB_HOME：
//   - Linux:   ~/.config/OKB
//   - macOS:   ~/Library/Application Support/OKB
//   - Windows: %AppData%\OKB
package config

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// Config holds all app configuration.
//
// Bootstrap 流程（见 internal/okb/bootstrap.go）：
//
//	uv tool install --prerelease=allow --from <OKBSpec> openkb
//
// 装好后会把 OpenKBBin、OpenKBPython 两个绝对路径填进来，后续 spawn 子进程都直接走绝对路径，
// 不再依赖 cwd / PATH / OpenKB 源码目录。
type Config struct {
	// 应用基础路径
	OKBHome    string `json:"-"` // <userConfigDir>/OKB；不持久化（自身就是 home）
	SpacesRoot string `json:"spaces_root,omitempty"`
	CacheDir   string `json:"-"` // 派生于 OKBHome
	RuntimeDir string `json:"-"` // 派生于 OKBHome

	// HTTP 服务
	Port string `json:"port,omitempty"`

	// OpenKB 引导
	OKBCmd       string `json:"-"` // uv 可执行文件路径，运行时探测
	OKBSpec      string `json:"okb_spec,omitempty"`
	OpenKBBin    string `json:"-"` // 装好后的 openkb 命令绝对路径（bootstrap 后填）
	OpenKBPython string `json:"-"` // 装好后该 venv 里的 python 绝对路径

	// LLM（用户在 Web 设置页可改，写到 config.json）
	LLMApiKey    string `json:"llm_api_key,omitempty"`
	LLMBaseURL   string `json:"llm_base_url,omitempty"`
	LLMModel     string `json:"llm_model,omitempty"`
	LLMLanguage  string `json:"llm_language,omitempty"`
	LLMAuxModel  string `json:"llm_aux_model,omitempty"` // 辅助模型：fact-check / follow-ups 等轻量任务（如 flash）
}

// C 全局唯一实例。读多写少，访问加 mu 锁；通过 Snapshot()/Update() 来读写。
var (
	C  Config
	mu sync.RWMutex
)

// 默认从 OpenKB main 分支装（含 deck 命令；PyPI 0.3.0 不带 deck）。
const defaultOKBSpec = "git+https://github.com/VectifyAI/OpenKB"

// Init 装载 .env、读 config.json、定 OKBHome，然后 mkdir 必要目录。
//
// 优先级：env > config.json > 默认。
// 写回 config.json：仅 Save() 显式调用（Web 设置页保存时）。
func Init() {
	godotenv.Load()

	home := resolveOKBHome()
	cfgPath := filepath.Join(home, "config.json")

	cfg := Config{
		OKBHome:    home,
		CacheDir:   filepath.Join(home, "cache"),
		RuntimeDir: filepath.Join(home, "runtime"),
		// LLM 默认值，下面被 disk 配置覆盖，再被 env 覆盖
		LLMBaseURL:  "https://api.deepseek.com",
		LLMModel:    "deepseek/deepseek-chat",
		LLMLanguage: "zh",
	}

	// 1) 读 config.json（用户上次在 Web 设置页保存的）
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(data, &cfg) // 失败时保留默认值，不致命
	}

	// 2) env 兜底：LLM 相关字段 Web 设置页保存过的优先（config.json > env > 默认）
	// PORT / SPACES_ROOT / OKB_SPEC 保持 env 最高优先级（基础设施）
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	} else if cfg.Port == "" {
		cfg.Port = "8901"
	}
	if v := os.Getenv("SPACES_ROOT"); v != "" {
		cfg.SpacesRoot = expandHome(v)
	} else if cfg.SpacesRoot == "" {
		cfg.SpacesRoot = filepath.Join(home, "spaces")
	} else {
		cfg.SpacesRoot = expandHome(cfg.SpacesRoot)
	}
	if v := os.Getenv("OKB_SPEC"); v != "" {
		cfg.OKBSpec = v
	} else if cfg.OKBSpec == "" {
		cfg.OKBSpec = defaultOKBSpec
	}
	if cfg.LLMApiKey == "" && os.Getenv("LLM_API_KEY") != "" {
		cfg.LLMApiKey = os.Getenv("LLM_API_KEY")
	}
	if cfg.LLMBaseURL == "" || cfg.LLMBaseURL == "https://api.deepseek.com" {
		if v := os.Getenv("LLM_BASE_URL"); v != "" {
			cfg.LLMBaseURL = v
		}
	}
	if cfg.LLMModel == "" || cfg.LLMModel == "deepseek/deepseek-chat" {
		if v := os.Getenv("LLM_MODEL"); v != "" {
			cfg.LLMModel = v
		}
	}
	if cfg.LLMLanguage == "" && os.Getenv("LLM_LANGUAGE") != "" {
		cfg.LLMLanguage = os.Getenv("LLM_LANGUAGE")
	}

	// 3) 旧版兼容：cwd 下有 okb-spaces 且 SpacesRoot 落在默认位置时，优先用 cwd 那个
	if v := os.Getenv("SPACES_ROOT"); v == "" {
		if cwd, err := os.Getwd(); err == nil {
			legacy := filepath.Join(cwd, "okb-spaces")
			if info, err := os.Stat(legacy); err == nil && info.IsDir() {
				log.Printf("ℹ️  detected legacy ./okb-spaces, using it (set SPACES_ROOT to override)")
				cfg.SpacesRoot = legacy
			}
		}
	}

	// 4) 探测 uv（运行时，不持久化）
	cfg.OKBCmd = resolveUv()

	// 5) mkdir 必要目录
	_ = os.MkdirAll(cfg.OKBHome, 0o755)
	_ = os.MkdirAll(cfg.CacheDir, 0o755)
	_ = os.MkdirAll(cfg.RuntimeDir, 0o755)
	_ = os.MkdirAll(cfg.SpacesRoot, 0o755)

	mu.Lock()
	C = cfg
	mu.Unlock()

	if cfg.OKBCmd == "" {
		log.Printf("⚠️  未找到 uv，请先安装：curl -LsSf https://astral.sh/uv/install.sh | sh")
	}
	log.Printf("🏠 OKB home:    %s", cfg.OKBHome)
	log.Printf("📁 spaces:      %s", cfg.SpacesRoot)
	log.Printf("🗂️  cache:       %s", cfg.CacheDir)
	log.Printf("📦 OpenKB spec: %s", cfg.OKBSpec)
}

// Snapshot 返回当前配置的副本（读专用，避免外部修改全局 C）。
func Snapshot() Config {
	mu.RLock()
	defer mu.RUnlock()
	return C
}

// normalizeModel 确保 model 带 provider 前缀（LiteLLM 要求 provider/model 格式）。
// 已有 "/" 前缀直接返回；否则按 baseURL 域名特征推断 provider，兜底 deepseek。
func normalizeModel(model, baseURL string) string {
	if strings.Contains(model, "/") {
		return model
	}
	u := strings.ToLower(baseURL)
	switch {
	case strings.Contains(u, "deepseek"):
		return "deepseek/" + model
	case strings.Contains(u, "openai"):
		return "openai/" + model
	case strings.Contains(u, "anthropic"):
		return "anthropic/" + model
	case strings.Contains(u, "gemini") || strings.Contains(u, "googleapis"):
		return "gemini/" + model
	case strings.Contains(u, "groq"):
		return "groq/" + model
	default:
		return "deepseek/" + model
	}
}

// Save 把传入的 patch 应用到全局 C 并写盘 config.json。
// 仅持久化 patch 里非空的字段（避免用户只改 LLMApiKey 时把 Port 也清掉）。
//
// 特殊：字段值 "__CLEAR__" 表示"显式清空"（让用户可以从 Web 设置页删 API key）。
//
// 调用方：Web 设置页 POST /api/settings handler。
func Save(patch Config) error {
	const clear = "__CLEAR__"

	mu.Lock()
	if patch.SpacesRoot == clear {
		C.SpacesRoot = ""
	} else if patch.SpacesRoot != "" {
		C.SpacesRoot = expandHome(patch.SpacesRoot)
	}
	if patch.Port == clear {
		C.Port = ""
	} else if patch.Port != "" {
		C.Port = patch.Port
	}
	if patch.OKBSpec == clear {
		C.OKBSpec = ""
	} else if patch.OKBSpec != "" {
		C.OKBSpec = patch.OKBSpec
	}
	if patch.LLMApiKey == clear {
		C.LLMApiKey = ""
	} else if patch.LLMApiKey != "" {
		C.LLMApiKey = patch.LLMApiKey
	}
	if patch.LLMBaseURL == clear {
		C.LLMBaseURL = ""
	} else if patch.LLMBaseURL != "" {
		C.LLMBaseURL = patch.LLMBaseURL
	}
	if patch.LLMModel == clear {
		C.LLMModel = ""
	} else if patch.LLMModel != "" {
		C.LLMModel = normalizeModel(patch.LLMModel, C.LLMBaseURL)
	}
	if patch.LLMLanguage == clear {
		C.LLMLanguage = ""
	} else if patch.LLMLanguage != "" {
		C.LLMLanguage = patch.LLMLanguage
	}
	if patch.LLMAuxModel == clear {
		C.LLMAuxModel = ""
	} else if patch.LLMAuxModel != "" {
		C.LLMAuxModel = normalizeModel(patch.LLMAuxModel, C.LLMBaseURL)
	}
	cfg := C
	mu.Unlock()

	cfgPath := filepath.Join(cfg.OKBHome, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, cfgPath)
}

// resolveOKBHome 决定 OKB_HOME 路径。优先级：env > UserConfigDir > UserHomeDir/.OKB
func resolveOKBHome() string {
	if v := os.Getenv("OKB_HOME"); v != "" {
		return expandHome(v)
	}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "OKB")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".OKB")
}

// resolveUv 按优先级查找 uv 可执行文件：
//  1. UV_BIN 环境变量
//  2. <OKBHome>/runtime/bin/uv （vendored Python 引导后会装在这里）
//  3. PATH 中的 uv（exec.LookPath）
//  4. ~/.local/bin/uv（uv 官方安装脚本默认位置）
//  5. /usr/local/bin/uv、/opt/homebrew/bin/uv
//
// 找不到返回空串，bootstrap 会向用户报错（或触发首次初始化下载）。
func resolveUv() string {
	if v := os.Getenv("UV_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	// 检查我们自己 vendored 的 uv
	if home := os.Getenv("OKB_HOME"); home != "" {
		// 注意：此时 C 还没初始化好，直接用 env 算
		for _, sub := range []string{"runtime/bin/uv", "runtime/bin/uv.exe"} {
			c := filepath.Join(expandHome(home), sub)
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
	}
	if p, err := exec.LookPath("uv"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		filepath.Join(home, ".local/bin/uv"),
		filepath.Join(home, ".local/bin/uv.exe"),
		"/usr/local/bin/uv",
		"/opt/homebrew/bin/uv",
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// envOr 保留：handler / okb 等子包仍可能用到（运行时透传）。
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// expandHome 展开 "~" 前缀。
func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if len(path) > 1 && path[0] == '~' && (path[1] == '/' || path[1] == filepath.Separator) {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

var _ = envOr // 保留导出
