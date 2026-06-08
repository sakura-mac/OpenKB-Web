// Package watch 监听每个 space 的 raw/ 目录，文件改动时自动 spawn `openkb add`/`openkb remove`，
// 让用户手动放文件到 raw/ 也能触发编译，不必走前端上传 API。
//
// 设计：
//   - 每个 space 一个 watcher（fsnotify.Watcher 内部 inotify/kqueue/etc）
//   - 事件级别 debounce：每个 path 各自有一个 timer，最后一次事件后 2s 才触发动作
//     （用户连续 cp/mv 一堆文件时不会跑 100 次 add）
//   - 跳过 dotfiles（.DS_Store / .partial / 编辑器临时文件）和目录事件
//   - Create / Write / Rename(到本目录) → spawn `openkb add <path>`
//   - Remove / Rename(离开本目录) → spawn `openkb remove <basename>`
//   - 每个动作进 task 队列（okb.NewTask + UpdateTask），前端能看到"自动编译中"
//
// 不做：
//   - 不递归子目录（OpenKB 也不递归）
//   - 不重启失败的 add（让 task error 显示给用户，他自己决定）
//   - 不去重相邻 Write（fsnotify 单次保存可能多个 Write，debounce 把它们合一次）
package watch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"okb-web/internal/okb"
)

// debounce 时长：用户连续操作（cp -r、git checkout）后多久没新事件才落地动作
const debounceDuration = 2 * time.Second

// Manager 管所有 space 的 watcher 生命周期。一般在进程启动时初始化一个全局实例。
type Manager struct {
	mu       sync.Mutex
	watchers map[string]*spaceWatcher // key = spaceDir 绝对路径

	// 忽略列表：upload/add handler 自己 spawn `openkb add` 时，先把目标 path 加进来，
	// 等 add 完成再清理。避免 watcher 对同一个路径再触发一次 add。
	// 用 path 字符串绝对值作 key（filepath.Clean 过）。
	ignoreMu sync.Mutex
	ignore   map[string]int // path -> refcount，多次 Ignore 同 path 也安全
}

// spaceWatcher 对应单个 space。包含 fsnotify watcher、debounce timer 表、stop chan。
type spaceWatcher struct {
	mgr      *Manager
	spaceDir string
	rawDir   string
	w        *fsnotify.Watcher

	// 每个 path 一个 debounce timer（避免单个文件多次 Write 都触发）
	timersMu sync.Mutex
	timers   map[string]*time.Timer
	// 记录最后一次事件 op：决定 timer 触发后是 add 还是 remove
	lastOp map[string]fsnotify.Op

	stop chan struct{}
	once sync.Once
}

// NewManager 构造空 Manager。
func NewManager() *Manager {
	return &Manager{
		watchers: make(map[string]*spaceWatcher),
		ignore:   make(map[string]int),
	}
}

// Ignore 临时屏蔽 path 的 watch 事件，返回 unignore 函数。
//
// 用法：upload handler 把文件写到 raw/ 时调一次 unignore := wm.Ignore(path)，
// 等 `openkb add path` 调完再 defer unignore()。这样 fsnotify 触发的 Create/Write
// 事件会被 watcher 静默跳过，避免对同一个文件跑两遍 add。
//
// **延迟解除**：unignore() 不立即从 map 删，而是延迟 debounceDuration + 1s。
// 因为 watcher 的事件 → debounce timer → timer 到期才查 isIgnored，整个链路最长
// debounceDuration。如果 handler 跑得比 add 还快（不可能但保险），立即解除会让
// timer 到期时 isIgnored=false → 重复 add。延迟解除保证 timer 一定看得到 ignore。
//
// refcount：同一个 path 多次 Ignore 也安全（理论上不应发生，但保险）。
func (m *Manager) Ignore(path string) func() {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	m.ignoreMu.Lock()
	m.ignore[abs]++
	m.ignoreMu.Unlock()
	return func() {
		// 延迟解除：让 fsnotify 事件 + debounce timer 一定能看到 ignore 状态
		time.AfterFunc(debounceDuration+time.Second, func() {
			m.ignoreMu.Lock()
			defer m.ignoreMu.Unlock()
			if c, ok := m.ignore[abs]; ok {
				if c <= 1 {
					delete(m.ignore, abs)
				} else {
					m.ignore[abs] = c - 1
				}
			}
		})
	}
}

// isIgnored 内部查询：watcher 主循环用它判断是否跳过事件。
func (m *Manager) isIgnored(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	m.ignoreMu.Lock()
	defer m.ignoreMu.Unlock()
	_, ok := m.ignore[abs]
	return ok
}

// EnsureSpace 保证给定 space 的 raw/ 已经在监听。已存在则 no-op。
//
// raw/ 目录如果还不存在就先 mkdir（新 space 在 OpenKB init 之前可能没 raw）。
func (m *Manager) EnsureSpace(spaceDir string) error {
	abs, err := filepath.Abs(spaceDir)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.watchers[abs]; ok {
		return nil
	}

	rawDir := filepath.Join(abs, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return fmt.Errorf("mkdir raw: %w", err)
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new fsnotify watcher: %w", err)
	}
	if err := w.Add(rawDir); err != nil {
		w.Close()
		return fmt.Errorf("watch raw dir: %w", err)
	}

	sw := &spaceWatcher{
		mgr:      m,
		spaceDir: abs,
		rawDir:   rawDir,
		w:        w,
		timers:   make(map[string]*time.Timer),
		lastOp:   make(map[string]fsnotify.Op),
		stop:     make(chan struct{}),
	}
	m.watchers[abs] = sw

	go sw.run()
	log.Printf("👁  watch raw/: %s", rawDir)
	return nil
}

// RemoveSpace 停止某 space 的 watcher（用于删除 space 时清理）。
func (m *Manager) RemoveSpace(spaceDir string) {
	abs, _ := filepath.Abs(spaceDir)
	m.mu.Lock()
	sw, ok := m.watchers[abs]
	if ok {
		delete(m.watchers, abs)
	}
	m.mu.Unlock()
	if ok {
		sw.close()
	}
}

// Close 关闭所有 watcher（进程退出时调）。
func (m *Manager) Close() {
	m.mu.Lock()
	all := m.watchers
	m.watchers = make(map[string]*spaceWatcher)
	m.mu.Unlock()
	for _, sw := range all {
		sw.close()
	}
}

func (sw *spaceWatcher) close() {
	sw.once.Do(func() {
		close(sw.stop)
		_ = sw.w.Close()
		// 让 pending timers 静默过期
		sw.timersMu.Lock()
		for _, t := range sw.timers {
			t.Stop()
		}
		sw.timers = nil
		sw.timersMu.Unlock()
	})
}

// run 是 fsnotify 事件主循环。每个 event 都重置或新建 path 对应的 debounce timer，
// 最后一次事件后 debounceDuration 没新事件，timer 触发 → 真正动作。
func (sw *spaceWatcher) run() {
	for {
		select {
		case <-sw.stop:
			return
		case err, ok := <-sw.w.Errors:
			if !ok {
				return
			}
			log.Printf("⚠️  watch err [%s]: %v", sw.rawDir, err)
		case ev, ok := <-sw.w.Events:
			if !ok {
				return
			}
			if !sw.shouldHandle(ev) {
				continue
			}
			sw.scheduleDebounced(ev)
		}
	}
}

// shouldHandle 过滤事件：跳过 dotfile / 目录 / 临时文件 / 显式忽略列表（upload handler 占用）。
func (sw *spaceWatcher) shouldHandle(ev fsnotify.Event) bool {
	// 显式忽略：上层 handler 自己 spawn add 时占用，跳过避免重复
	if sw.mgr.isIgnored(ev.Name) {
		return false
	}
	base := filepath.Base(ev.Name)
	if base == "" || strings.HasPrefix(base, ".") {
		return false
	}
	// 编辑器/下载工具的临时文件后缀
	for _, suf := range []string{".tmp", ".swp", ".swx", ".part", ".partial", ".crdownload", "~"} {
		if strings.HasSuffix(base, suf) {
			return false
		}
	}
	// Chmod 单独事件不动作（数据没变）
	if ev.Op == fsnotify.Chmod {
		return false
	}
	// Create 时如果是目录事件，跳过（OpenKB 不递归）
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			return false
		}
	}
	return true
}

// scheduleDebounced 为该 path 重置/新建 timer；记录"最近一次 op"用于触发时决定 add/remove。
func (sw *spaceWatcher) scheduleDebounced(ev fsnotify.Event) {
	sw.timersMu.Lock()
	defer sw.timersMu.Unlock()
	if sw.timers == nil {
		return // 已关闭
	}
	if t, ok := sw.timers[ev.Name]; ok {
		t.Stop()
	}
	sw.lastOp[ev.Name] = ev.Op
	path := ev.Name
	sw.timers[path] = time.AfterFunc(debounceDuration, func() {
		sw.timersMu.Lock()
		op := sw.lastOp[path]
		delete(sw.timers, path)
		delete(sw.lastOp, path)
		sw.timersMu.Unlock()
		sw.performAction(path, op)
	})
}

// performAction 真正执行：对 path 调 openkb add 或 openkb remove。
// remove 路径用 basename（OpenKB remove 接受文件名）。
func (sw *spaceWatcher) performAction(path string, op fsnotify.Op) {
	// 删除 / 重命名出去 → openkb remove
	if op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		// 文件已不存在；用 basename 调 openkb remove
		if _, err := os.Stat(path); err != nil {
			sw.spawnRemove(filepath.Base(path))
			return
		}
		// rename 进来同名（罕见）：fall through 到 add 路径
	}
	// 文件还在 → 当作 add（Create / Write）
	if _, err := os.Stat(path); err != nil {
		// 已不在了，又不是 remove 事件——可能 race，跳过
		return
	}
	sw.spawnAdd(path)
}

// spawnAdd 后台跑 `openkb add <path>`，进 task 队列。
func (sw *spaceWatcher) spawnAdd(path string) {
	base := filepath.Base(path)
	spaceName := filepath.Base(sw.spaceDir)
	taskID := okb.NewTask(spaceName, 1)
	okb.UpdateTask(taskID, "running", fmt.Sprintf("自动编译：%s", base), nil)
	go func() {
		success, _, stderr := okb.Run([]string{"add", path}, sw.spaceDir)
		if success {
			okb.UpdateTask(taskID, "done", fmt.Sprintf("已编译：%s", base), []string{base})
			log.Printf("✓ auto-add %s", path)
		} else {
			msg := stderr
			if len(msg) > 400 {
				msg = msg[:400] + "..."
			}
			okb.UpdateTask(taskID, "error", fmt.Sprintf("编译失败：%s\n%s", base, msg), nil)
			log.Printf("✗ auto-add %s: %s", path, stderr)
		}
	}()
}

// spawnRemove 后台跑 `openkb remove <basename>`，进 task 队列。
func (sw *spaceWatcher) spawnRemove(base string) {
	spaceName := filepath.Base(sw.spaceDir)
	taskID := okb.NewTask(spaceName, 1)
	okb.UpdateTask(taskID, "running", fmt.Sprintf("自动移除：%s", base), nil)
	go func() {
		success, _, stderr := okb.Run([]string{"remove", base}, sw.spaceDir)
		if success {
			okb.UpdateTask(taskID, "done", fmt.Sprintf("已移除：%s", base), []string{base})
			log.Printf("✓ auto-remove %s", base)
		} else {
			msg := stderr
			if len(msg) > 400 {
				msg = msg[:400] + "..."
			}
			okb.UpdateTask(taskID, "error", fmt.Sprintf("移除失败：%s\n%s", base, msg), nil)
			log.Printf("✗ auto-remove %s: %s", base, stderr)
		}
	}()
}
