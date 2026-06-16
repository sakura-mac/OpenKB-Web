package okb

import (
	"os/exec"
	"strings"
	"sync"

	"okb-web/internal/config"
)

// SpaceLock 返回指定 space 的互斥锁，不同 space 独立。
// 同一 space 的 add/remove/recompile 必须串行，否则：
//  1. .git/index.lock 抢锁失败导致 commit 丢失
//  2. OpenKB 内部的 wiki/ 写文件也可能被并发改坏
var (
	spaceLocksMu sync.Mutex
	spaceLocks   = make(map[string]*sync.Mutex)
)

func SpaceLock(space string) *sync.Mutex {
	spaceLocksMu.Lock()
	defer spaceLocksMu.Unlock()
	if lk, ok := spaceLocks[space]; ok {
		return lk
	}
	lk := &sync.Mutex{}
	spaceLocks[space] = lk
	return lk
}

// Run executes an openkb command and returns (success, stdout, stderr).
//
// 直接调 bootstrap 时探测好的 OpenKBBin 绝对路径，省一层 `uv run` 启动开销，
// 也彻底摆脱 cwd / PATH 依赖。
func Run(args []string, workDir string) (bool, string, string) {
	return RunWithStdin(args, workDir, "")
}

// RunWithStdin 执行 openkb 命令，带可选 stdin 输入。
//
// stdin 处理特别说明（修过的坑）：
//   - 之前用 strings.NewReader(stdinInput)：N 次 read 之后就 EOF。
//     openkb init 内部会发若干个 input() prompt（确认 model / language / pageindex），
//     单个 "\n" 喂完后第二次 input() read 到 EOF 就抛 EOFError 让 init 失败；
//     但如果父进程是 terminal，子进程 stdin 偶尔会"穿透"读到父 terminal——
//     表现就是"在终端打回车才创建成功"那个奇异现象。
//   - 现在策略：把 stdinInput 复制 16 份（足够覆盖任何已知 prompt 数量），
//     再 close stdin → 之后 read 立刻 EOF（cmd.Stdin = io.Reader 隐含此行为）。
//     即使是 GUI/Setup 启动（无 terminal），子进程也不会 hang。
//   - 对 stdinInput=="" 的情况：显式 set Stdin 为 /dev/null（Win 上 NUL）等价物，
//     防止子进程从父进程 terminal 偷读。
func RunWithStdin(args []string, workDir, stdinInput string) (bool, string, string) {
	cmd := exec.Command(config.C.OpenKBBin, args...)
	cmd.Dir = workDir

	if stdinInput == "" {
		// 没 stdin → 用空 reader（read 立即 EOF），不让子进程读父 terminal。
		// 否则 openkb 子命令意外触发 input() 时，会从用户的实际 terminal 偷读，
		// 表现成"程序卡住直到我按了回车"。
		cmd.Stdin = strings.NewReader("")
	} else {
		// 重复 16 次，覆盖 openkb init 等多 prompt 流程。
		// 16 远超实际 prompt 数（~3）；多余的 read 不影响（init 早就 return 了）。
		cmd.Stdin = strings.NewReader(strings.Repeat(stdinInput, 16))
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return err == nil, stdout.String(), stderr.String()
}
