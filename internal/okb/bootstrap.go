package okb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"okb-web/internal/assets"
	"okb-web/internal/config"
)

// Bootstrap 在 okb-web 启动早期被调用，做三件事：
//
//  1. 确保 OpenKB 已用 `uv tool install --from <OKBSpec> openkb` 装好
//     （幂等：把 spec 的 SHA 存到 <cacheDir>/installed-spec.sha256，spec 没变就跳过）
//  2. 解析装好之后 openkb 二进制和该 venv 内 python 的绝对路径，
//     存入 config.C.OpenKBBin / config.C.OpenKBPython
//  3. 释放 chat_helper.py 与 skills 到 cacheDir，让后续 chat / deck 都用绝对路径调
//
// 任意一步失败都返回有用的错误（不崩溃，由调用方决定是 fatal 还是降级）。
func Bootstrap() error {
	if config.C.OKBCmd == "" {
		return fmt.Errorf("未找到 uv，请先安装：curl -LsSf https://astral.sh/uv/install.sh | sh")
	}

	// 1) install openkb (幂等)
	if err := ensureOpenKB(); err != nil {
		return err
	}

	// 2) 解析 openkb / python 路径
	bin, py, err := locateOpenKBBins()
	if err != nil {
		return fmt.Errorf("openkb 装好了但找不到可执行文件：%w", err)
	}
	config.C.OpenKBBin = bin
	config.C.OpenKBPython = py

	// 3) 释放静态资源
	helper, err := assets.ChatHelperPath(config.C.CacheDir)
	if err != nil {
		return fmt.Errorf("释放 chat_helper.py 失败：%w", err)
	}
	skills, err := assets.SkillsDir(config.C.CacheDir)
	if err != nil {
		return fmt.Errorf("释放 skills 失败：%w", err)
	}

	fmt.Printf("✅ OpenKB ready\n   bin:    %s\n   python: %s\n   helper: %s\n   skills: %s\n",
		bin, py, helper, skills)
	return nil
}

// ensureOpenKB: 如果已按当前 spec 装过则跳过，否则跑 uv tool install。
func ensureOpenKB() error {
	specSha := sha256.Sum256([]byte(config.C.OKBSpec))
	specHash := hex.EncodeToString(specSha[:])
	marker := filepath.Join(config.C.CacheDir, "installed-spec.sha256")

	if existing, err := os.ReadFile(marker); err == nil {
		if strings.TrimSpace(string(existing)) == specHash {
			// 还要确认 openkb 二进制确实存在（用户可能手动删了 ~/.local/share/uv/tools）
			if _, _, err := locateOpenKBBins(); err == nil {
				return nil
			}
		}
	}

	fmt.Printf("⏳ 首次启动：用 uv 安装 OpenKB（%s）...\n", config.C.OKBSpec)
	start := time.Now()

	args := []string{
		"tool", "install",
		"--force",
		"--prerelease=allow",
		"--from", config.C.OKBSpec,
		"openkb",
	}
	cmd := exec.Command(config.C.OKBCmd, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv tool install 失败：%w（spec=%s）", err, config.C.OKBSpec)
	}

	if err := os.WriteFile(marker, []byte(specHash), 0o644); err != nil {
		// 非致命：下次会重装一次，浪费 30s 但不影响正确性
		fmt.Printf("⚠️  写 installed-spec marker 失败：%v\n", err)
	}
	fmt.Printf("✅ OpenKB 安装完成（耗时 %ds）\n", int(time.Since(start).Seconds()))
	return nil
}

// locateOpenKBBins 找到 uv 装好的 openkb 命令和它所在 venv 里的 python。
//
// 优先用 `uv tool dir openkb` 拿绝对路径（uv 0.5+ 支持），
// 失败则回退到默认布局 ~/.local/share/uv/tools/openkb/。
func locateOpenKBBins() (string, string, error) {
	// 方案 A: uv tool dir openkb（输出 venv 根目录）
	out, err := exec.Command(config.C.OKBCmd, "tool", "dir", "openkb").Output()
	venvDir := strings.TrimSpace(string(out))
	if err != nil || venvDir == "" {
		// 方案 B: 默认布局
		home, _ := os.UserHomeDir()
		venvDir = filepath.Join(home, ".local/share/uv/tools/openkb")
	}

	// venv 布局：bin/openkb、bin/python（Linux/Mac）或 Scripts/openkb.exe（Win）
	binDir := filepath.Join(venvDir, "bin")
	if _, err := os.Stat(binDir); err != nil {
		binDir = filepath.Join(venvDir, "Scripts") // Windows
	}

	openkbBin := filepath.Join(binDir, "openkb")
	if _, err := os.Stat(openkbBin); err != nil {
		// Windows 后缀
		if _, err2 := os.Stat(openkbBin + ".exe"); err2 == nil {
			openkbBin = openkbBin + ".exe"
		} else {
			return "", "", fmt.Errorf("openkb 命令不存在：%s", openkbBin)
		}
	}

	pyBin := filepath.Join(binDir, "python")
	if _, err := os.Stat(pyBin); err != nil {
		// 试 python3 / python.exe
		for _, alt := range []string{"python3", "python.exe", "python3.exe"} {
			if _, err := os.Stat(filepath.Join(binDir, alt)); err == nil {
				pyBin = filepath.Join(binDir, alt)
				break
			}
		}
	}
	if _, err := os.Stat(pyBin); err != nil {
		return "", "", fmt.Errorf("python 不存在：%s", pyBin)
	}

	return openkbBin, pyBin, nil
}
