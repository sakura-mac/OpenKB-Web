package config

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"

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
	Port        string
	SpacesRoot  string
	CacheDir    string // ~/.cache/okb-web，用于释放 chat_helper.py 与 skills
	OKBCmd      string // uv 可执行文件路径
	OKBSpec     string // 给 uv tool install 用的 spec：默认 git+url，可改成 openkb==0.4.0
	OpenKBBin   string // 装好后的 openkb 命令绝对路径（bootstrap 后填）
	OpenKBPython string // 装好后该 venv 里的 python 绝对路径（chat_helper 用）
	LLMApiKey   string
	LLMBaseURL  string
	LLMModel    string
	LLMLanguage string
}

// C is the global config instance.
var C Config

// 默认从 OpenKB main 分支装（含 deck 命令；PyPI 0.3.0 不带 deck）。
// 等 0.4.0 发布且带 deck，可改为 "openkb==0.4.0"。
const defaultOKBSpec = "git+https://github.com/VectifyAI/OpenKB"

// Init loads .env and populates C.
func Init() {
	godotenv.Load()

	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".cache", "okb-web")
	_ = os.MkdirAll(cacheDir, 0o755)

	C = Config{
		Port:        envOr("PORT", "8901"),
		SpacesRoot:  expandHome(envOr("SPACES_ROOT", "okb-spaces")),
		CacheDir:    cacheDir,
		OKBCmd:      resolveUv(),
		OKBSpec:     envOr("OKB_SPEC", defaultOKBSpec),
		LLMApiKey:   os.Getenv("LLM_API_KEY"),
		LLMBaseURL:  envOr("LLM_BASE_URL", "https://api.deepseek.com"),
		LLMModel:    envOr("LLM_MODEL", "deepseek/deepseek-chat"),
		LLMLanguage: envOr("LLM_LANGUAGE", "zh"),
	}

	if C.OKBCmd == "" {
		log.Printf("⚠️  未找到 uv，请先安装：curl -LsSf https://astral.sh/uv/install.sh | sh")
	}
	log.Printf("📦 OpenKB spec: %s", C.OKBSpec)
	log.Printf("🗂️  cache dir:  %s", C.CacheDir)

	_ = os.MkdirAll(C.SpacesRoot, 0o755)
}

// resolveUv 按优先级查找 uv 可执行文件：
//  1. UV_BIN 环境变量
//  2. PATH 中的 uv（exec.LookPath）
//  3. ~/.local/bin/uv（uv 官方安装脚本默认位置）
//  4. /usr/local/bin/uv、/opt/homebrew/bin/uv
//
// 找不到返回空串，bootstrap 会向用户报错。
func resolveUv() string {
	if v := os.Getenv("UV_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	if p, err := exec.LookPath("uv"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, c := range []string{
		filepath.Join(home, ".local/bin/uv"),
		"/usr/local/bin/uv",
		"/opt/homebrew/bin/uv",
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func expandHome(path string) string {
	if len(path) > 1 && path[0] == '~' && path[1] == '/' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
