package okb

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"okb-web/internal/assets"
	"okb-web/internal/config"
)

// Bootstrap 在 okb-web 启动早期被调用，做四件事（任意一步失败 → 状态置 PhaseFailed）：
//
//  1. 没 uv 则自动下载 standalone uv 到 <OKBHome>/runtime/bin/uv
//  2. 用 uv tool install 装 OpenKB 到 <OKBHome>/runtime/openkb-tool
//     （幂等：把 spec 的 SHA 存到 <CacheDir>/installed-spec.sha256，spec 没变就跳过）
//  3. 解析装好之后 openkb 二进制和该 venv 内 python 的绝对路径，
//     存入 config.C.OpenKBBin / config.C.OpenKBPython
//  4. 释放 chat_helper.py 与 skills 到 cacheDir
//
// 所有阶段通过 updateStatus(...) 推进，前端轮询 /api/bootstrap/status 看进度。
//
// 旧 API：保持 Bootstrap() 同步调用兼容（main.go 早期同步调用）。
// 新 API：BootstrapAsync()，go 一个 goroutine 跑，进程立即继续。
func Bootstrap() error {
	updateStatus(PhaseChecking, 5, "检查 OpenKB 引擎状态...")

	// 1) uv：缺失自动下载到 vendored 位置
	uvPath := config.C.OKBCmd
	if uvPath == "" {
		log.Printf("📥 未找到 uv，自动下载到 vendored 位置...")
		var err error
		uvPath, err = downloadUv()
		if err != nil {
			failStatus(err)
			return err
		}
		// 把 vendored uv 注回 config，后续 ensureOpenKB 用
		config.C.OKBCmd = uvPath
	}

	// 2) install openkb (幂等)
	updateStatus(PhaseInstall, 40, "安装 OpenKB（首次约 30-90 秒）...")
	if err := ensureOpenKB(); err != nil {
		failStatus(err)
		return err
	}

	// 3) 解析 openkb / python 路径
	updateStatus(PhaseInstall, 85, "定位 OpenKB 可执行文件...")
	bin, py, err := locateOpenKBBins()
	if err != nil {
		err = fmt.Errorf("openkb 装好了但找不到可执行文件：%w", err)
		failStatus(err)
		return err
	}
	config.C.OpenKBBin = bin
	config.C.OpenKBPython = py

	// 4) 释放静态资源
	updateStatus(PhaseRelease, 92, "释放运行时资源...")
	helper, err := assets.ChatHelperPath(config.C.CacheDir)
	if err != nil {
		err = fmt.Errorf("释放 chat_helper.py 失败：%w", err)
		failStatus(err)
		return err
	}
	skills, err := assets.SkillsDir(config.C.CacheDir)
	if err != nil {
		err = fmt.Errorf("释放 skills 失败：%w", err)
		failStatus(err)
		return err
	}

	updateStatus(PhaseReady, 100, "就绪")
	fmt.Printf("✅ OpenKB ready\n   bin:    %s\n   python: %s\n   helper: %s\n   skills: %s\n",
		bin, py, helper, skills)
	return nil
}

// BootstrapAsync 异步跑 Bootstrap()，进程不阻塞。失败时状态会变 PhaseFailed，
// 前端能从 /api/bootstrap/status 看到 error 字段。
//
// 多次调用安全：内部用全局 status，第二次进来时如果当前是 ready 就直接返回；
// 如果是 failed 就重跑（让用户改完 spec 后能"重试"）。
func BootstrapAsync() {
	go func() {
		// 改了 OKBSpec 等参数后用户可以再点"重试"，所以允许从 failed 重跑
		s := GetStatus()
		if s.Phase == PhaseReady {
			return
		}
		if err := Bootstrap(); err != nil {
			log.Printf("⚠️  bootstrap 失败：%v", err)
		}
	}()
}

// ensureOpenKB: 如果已按当前 spec 装过则跳过，否则跑 uv tool install。
func ensureOpenKB() error {
	specSha := sha256.Sum256([]byte(config.C.OKBSpec))
	specHash := hex.EncodeToString(specSha[:])
	marker := filepath.Join(config.C.CacheDir, "installed-spec.sha256")

	if existing, err := os.ReadFile(marker); err == nil {
		if strings.TrimSpace(string(existing)) == specHash {
			// 还要确认 openkb 二进制确实存在（用户可能手动删了）
			if _, _, err := locateOpenKBBins(); err == nil {
				updateStatus(PhaseInstall, 80, "OpenKB 已安装（缓存命中）")
				return nil
			}
		}
	}

	updateStatus(PhaseInstall, 50, fmt.Sprintf("uv tool install %s...", abbrevSpec(config.C.OKBSpec)))
	start := time.Now()

	// 把 uv tool 安装目录指到 <OKBHome>/runtime/openkb-tool，与系统隔离
	toolDir := filepath.Join(config.C.RuntimeDir, "openkb-tool")
	_ = os.MkdirAll(toolDir, 0o755)

	args := []string{
		"tool", "install",
		"--force",
		"--prerelease=allow",
		"--from", config.C.OKBSpec,
		"openkb",
	}
	cmd := exec.Command(config.C.OKBCmd, args...)
	// 用 UV_TOOL_DIR 让 uv 把 openkb 装到我们的 runtime 目录，
	// 不污染用户的 ~/.local/share/uv/tools
	cmd.Env = append(os.Environ(), "UV_TOOL_DIR="+toolDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uv tool install 失败：%w（spec=%s）", err, config.C.OKBSpec)
	}

	if err := os.WriteFile(marker, []byte(specHash), 0o644); err != nil {
		// 非致命：下次会重装一次，浪费 30s 但不影响正确性
		log.Printf("⚠️  写 installed-spec marker 失败：%v", err)
	}
	updateStatus(PhaseInstall, 80, fmt.Sprintf("安装完成（耗时 %ds）", int(time.Since(start).Seconds())))
	return nil
}

// abbrevSpec 把长 spec（如 git+https://github.com/...）截短到 60 字符内。
func abbrevSpec(s string) string {
	if len(s) <= 60 {
		return s
	}
	return s[:30] + "..." + s[len(s)-25:]
}

// locateOpenKBBins 找到 uv 装好的 openkb 命令和它所在 venv 里的 python。
//
// 优先用 `uv tool dir openkb` 拿绝对路径，
// 失败则回退到默认布局 ~/.local/share/uv/tools/openkb/ 或 vendored runtime/openkb-tool/openkb。
func locateOpenKBBins() (string, string, error) {
	// 候选 venv 根：vendored runtime > uv tool dir > 默认布局
	var candidates []string
	candidates = append(candidates, filepath.Join(config.C.RuntimeDir, "openkb-tool", "openkb"))

	// 用 UV_TOOL_DIR 设过的话 uv tool dir 会返回正确目录
	envForUv := append(os.Environ(),
		"UV_TOOL_DIR="+filepath.Join(config.C.RuntimeDir, "openkb-tool"))
	cmd := exec.Command(config.C.OKBCmd, "tool", "dir", "openkb")
	cmd.Env = envForUv
	if out, err := cmd.Output(); err == nil {
		venvDir := strings.TrimSpace(string(out))
		if venvDir != "" {
			candidates = append(candidates, venvDir)
		}
	}

	home, _ := os.UserHomeDir()
	candidates = append(candidates,
		filepath.Join(home, ".local/share/uv/tools/openkb"),
	)

	for _, venvDir := range candidates {
		bin, py, err := locateInVenv(venvDir)
		if err == nil {
			return bin, py, nil
		}
	}
	return "", "", fmt.Errorf("openkb 未在任何已知位置找到（已查找 %d 个候选）", len(candidates))
}

// locateInVenv 在某个 venv 目录里找 openkb / python 可执行文件。
func locateInVenv(venvDir string) (string, string, error) {
	binDir := filepath.Join(venvDir, "bin")
	if _, err := os.Stat(binDir); err != nil {
		binDir = filepath.Join(venvDir, "Scripts") // Windows
		if _, err := os.Stat(binDir); err != nil {
			return "", "", fmt.Errorf("既没 bin/ 也没 Scripts/：%s", venvDir)
		}
	}

	openkbBin := filepath.Join(binDir, "openkb")
	if _, err := os.Stat(openkbBin); err != nil {
		if _, err2 := os.Stat(openkbBin + ".exe"); err2 == nil {
			openkbBin = openkbBin + ".exe"
		} else {
			return "", "", fmt.Errorf("openkb 命令不存在：%s", openkbBin)
		}
	}

	pyBin := filepath.Join(binDir, "python")
	if _, err := os.Stat(pyBin); err != nil {
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
