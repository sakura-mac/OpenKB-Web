package codegraph

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"okb-web/internal/config"
)

// Bin 返回可用的 codegraph CLI 绝对路径。
// 优先使用 OKB runtime 内的 vendored 版本；找不到则回退 PATH。
func Bin() string {
	for _, p := range runtimeCandidates() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("codegraph"); err == nil {
		return p
	}
	return ""
}

// Ensure 确保 codegraph CLI 可用。找不到时自动安装到 <OKB_HOME>/runtime/codegraph。
func Ensure(ctx context.Context) (string, error) {
	if bin := Bin(); bin != "" {
		return bin, nil
	}
	if err := install(ctx); err != nil {
		return "", err
	}
	if bin := Bin(); bin != "" {
		return bin, nil
	}
	return "", fmt.Errorf("codegraph 安装完成但未找到可执行文件")
}

func runtimeCandidates() []string {
	root := filepath.Join(config.C.RuntimeDir, "codegraph")
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(root, "current", "bin", "codegraph.exe"),
			filepath.Join(root, "current", "bin", "codegraph.cmd"),
			filepath.Join(root, "current", "bin", "codegraph"),
		}
	}
	return []string{
		filepath.Join(root, "bin", "codegraph"),
		filepath.Join(root, "current", "bin", "codegraph"),
	}
}

func install(ctx context.Context) error {
	root := filepath.Join(config.C.RuntimeDir, "codegraph")
	log.Printf("📥 installing CodeGraph runtime into %s", root)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	installCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		ps := fmt.Sprintf(`$env:CODEGRAPH_INSTALL_DIR=%q; irm https://raw.githubusercontent.com/colbymchenry/codegraph/main/install.ps1 | iex`, root)
		cmd = exec.CommandContext(installCtx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	} else {
		binDir := filepath.Join(root, "bin")
		_ = os.MkdirAll(binDir, 0o755)
		sh := fmt.Sprintf(`CODEGRAPH_INSTALL_DIR=%q CODEGRAPH_BIN_DIR=%q curl -fsSL https://raw.githubusercontent.com/colbymchenry/codegraph/main/install.sh | sh`, root, binDir)
		cmd = exec.CommandContext(installCtx, "sh", "-c", sh)
	}
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 1200 {
			msg = msg[len(msg)-1200:]
		}
		return fmt.Errorf("install codegraph failed: %w\n%s", err, msg)
	}
	return nil
}

func Run(ctx context.Context, workDir string, args ...string) (bool, string, string) {
	bin, err := Ensure(ctx)
	if err != nil {
		return false, "", err.Error()
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader("")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	return err == nil, stdout.String(), stderr.String()
}
