package handler

import (
	"okb-web/internal/watch"
)

// 全局 watch manager 引用，main.go 启动时通过 SetWatchManager 注入。
// 各 handler 在创建 / 删除 space 时调它来挂载/卸载 raw/ 监听。
var wm *watch.Manager

// SetWatchManager 由 main.go 启动时调用，注入全局 watch manager。
// 注入前的调用（极不可能）会被 helper 函数静默忽略。
func SetWatchManager(m *watch.Manager) { wm = m }

// ensureWatch 给 spaceDir 挂上 raw/ watcher。注入前 / 错误时静默——
// 监听是 nice-to-have，不影响主流程。
func ensureWatch(spaceDir string) {
	if wm == nil {
		return
	}
	_ = wm.EnsureSpace(spaceDir)
}

// removeWatch 卸下 spaceDir 的 watcher（删 space 时调）。
func removeWatch(spaceDir string) {
	if wm == nil {
		return
	}
	wm.RemoveSpace(spaceDir)
}

// ignoreRawPath 临时屏蔽 watcher 对 path 的事件，返回 unignore 函数。
//
// upload / add / remove handler 改动 raw/ 时调一次：
//
//	defer ignoreRawPath(dst)()
//	... 写文件 + spawn `openkb add` ...
//
// 防止 fsnotify 触发的事件让 watcher 又跑一遍 add，造成重复编译。
// wm 未注入时返回 no-op，调用方逻辑不变。
func ignoreRawPath(path string) func() {
	if wm == nil {
		return func() {}
	}
	return wm.Ignore(path)
}
