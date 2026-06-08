package okb

import (
	"sync"
	"time"
)

// Phase 表示 bootstrap 当前阶段。前端轮询时根据 phase 决定是显示遮罩、进度还是放行。
type Phase string

const (
	PhasePending  Phase = "pending"   // 进程刚起来，还没开始 bootstrap
	PhaseChecking Phase = "checking"  // 检查 uv / openkb 是否已安装
	PhaseDownloadUv Phase = "download-uv" // 没 uv，正在下载
	PhaseInstall  Phase = "installing" // uv tool install openkb 中（最长 30-90s）
	PhaseRelease  Phase = "releasing" // 释放 chat_helper.py / skills 到 cache
	PhaseReady    Phase = "ready"     // 全部就绪，前端可以正常用
	PhaseFailed   Phase = "failed"    // 失败，需要用户排查
)

// BootstrapStatus 是 /api/bootstrap/status 返回的状态快照。
type BootstrapStatus struct {
	Phase     Phase     `json:"phase"`
	Message   string    `json:"message"`
	Progress  int       `json:"progress"` // 0-100
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

var (
	statusMu sync.RWMutex
	status   = BootstrapStatus{Phase: PhasePending}
)

// GetStatus 给 handler 调，安全读当前状态。
func GetStatus() BootstrapStatus {
	statusMu.RLock()
	defer statusMu.RUnlock()
	return status
}

// updateStatus 内部用：bootstrap 流程各阶段调它推进状态。
//
// progress 单调递增（不要倒退），message 用于前端显示。
// 只有 PhaseFailed 路径会写 error 字段，并记 EndedAt。
func updateStatus(phase Phase, progress int, message string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if status.StartedAt.IsZero() && phase != PhasePending {
		status.StartedAt = time.Now()
	}
	status.Phase = phase
	status.Message = message
	if progress > status.Progress {
		status.Progress = progress
	}
	if phase == PhaseReady || phase == PhaseFailed {
		status.EndedAt = time.Now()
		if phase == PhaseReady {
			status.Progress = 100
			status.Error = ""
		}
	}
}

// failStatus 标记 bootstrap 失败，记录错误信息。
func failStatus(err error) {
	statusMu.Lock()
	defer statusMu.Unlock()
	status.Phase = PhaseFailed
	status.Error = err.Error()
	status.EndedAt = time.Now()
}

// IsReady 给其他子系统快速判断（chat handler 之类的可以用来兜底拒绝请求）。
func IsReady() bool {
	statusMu.RLock()
	defer statusMu.RUnlock()
	return status.Phase == PhaseReady
}
