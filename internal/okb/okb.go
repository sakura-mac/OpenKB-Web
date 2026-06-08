package okb

import (
	"os/exec"
	"strings"

	"okb-web/internal/config"
)

// Run executes an openkb command and returns (success, stdout, stderr).
//
// 直接调 bootstrap 时探测好的 OpenKBBin 绝对路径，省一层 `uv run` 启动开销，
// 也彻底摆脱 cwd / PATH 依赖。
func Run(args []string, workDir string) (bool, string, string) {
	return RunWithStdin(args, workDir, "")
}

// RunWithStdin executes an openkb command with optional stdin input.
func RunWithStdin(args []string, workDir, stdinInput string) (bool, string, string) {
	cmd := exec.Command(config.C.OpenKBBin, args...)
	cmd.Dir = workDir

	if stdinInput != "" {
		cmd.Stdin = strings.NewReader(stdinInput)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return err == nil, stdout.String(), stderr.String()
}
