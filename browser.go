package main

import (
	"net"
	"os/exec"
	"runtime"
	"time"
)

// openBrowser 跨平台打开默认浏览器到 url。
//
// 实现：纯标准库（不引 pkg/browser，省一个依赖）。
//   - macOS:   open <url>
//   - Windows: rundll32 url.dll,FileProtocolHandler <url>
//     （比 cmd /c start 更稳：start 把 url 中 & 当成命令分隔符会出问题）
//   - Linux:   xdg-open <url>（GNOME/KDE 都支持，新发行版必装）
//
// 失败静默：用户没有 GUI 桌面（headless server / SSH session）很正常，
// 不应该报错把进程搞挂。日志层面只在 main.go 里打一行"自动打开浏览器：..."
// 失败用户也能从终端 log 里看到 URL 自己复制粘贴。
//
// 调用方应该用 goroutine 调，避免阻塞主流程。
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux / *bsd
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start() // 不 Wait，浏览器进程脱离父进程
}

// waitPortReady 阻塞至 :port 可被 dial（HTTP 服务真正起来），
// 或 timeout 超时。给 openBrowser 用：HTTP 没起来就 open URL
// 浏览器会显示"无法连接"白屏，体验差。
//
// 用 50ms 步长轮询，TCP dial 已经是 µs 级，对启动延迟无感知。
func waitPortReady(port string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort("127.0.0.1", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
