package okb

import (
	"fmt"
	"sync"
	"time"
)

// Task represents an async background job.
type Task struct {
	ID        string    `json:"id"`
	Space     string    `json:"space"`
	Status    string    `json:"status"` // "running", "done", "error"
	Message   string    `json:"message"`
	Files     []string  `json:"files,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	tasks   = make(map[string]*Task)
	tasksMu sync.RWMutex
	taskSeq int
)

// NewTask creates a task and returns its ID.
func NewTask(space string, fileCount int) string {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	taskSeq++
	id := fmt.Sprintf("task_%d_%d", time.Now().Unix(), taskSeq)
	tasks[id] = &Task{
		ID:        id,
		Space:     space,
		Status:    "running",
		Message:   fmt.Sprintf("正在编译 %d 个文件...", fileCount),
		CreatedAt: time.Now(),
	}
	return id
}

// UpdateTask updates a task's status.
func UpdateTask(id, status, message string, files []string) {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	if t, ok := tasks[id]; ok {
		t.Status = status
		t.Message = message
		if files != nil {
			t.Files = files
		}
	}
}

// GetTask retrieves a task by ID.
func GetTask(id string) *Task {
	tasksMu.RLock()
	defer tasksMu.RUnlock()
	return tasks[id]
}

// CleanOldTasks 清理已完成且超过 30 分钟的 task。
// 仍在 running 的 task 永不被清，避免长任务（如 deck --critique）被误删。
func CleanOldTasks() {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	cutoff := time.Now().Add(-30 * time.Minute)
	for id, t := range tasks {
		if t.Status != "running" && t.CreatedAt.Before(cutoff) {
			delete(tasks, id)
		}
	}
}
