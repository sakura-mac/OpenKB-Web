package okb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitInit initializes a git repo in the space directory (idempotent).
// Returns error if git init fails (e.g. disk full, permission denied).
func GitInit(spaceDir string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = spaceDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w\n%s", err, out)
	}

	// Set user for commits
	if err := exec.Command("git", "-C", spaceDir, "config", "user.email", "okb-web@local").Run(); err != nil {
		return fmt.Errorf("git config user.email: %w", err)
	}
	if err := exec.Command("git", "-C", spaceDir, "config", "user.name", "OKB Web").Run(); err != nil {
		return fmt.Errorf("git config user.name: %w", err)
	}

	// Create .gitignore if not exists（直接写文件，避免 echo 不解析 \n 导致 .env 被纳入版本库）
	// output/ 是 deck/skill 等命令的产物目录，不应进版本库
	gitignore := filepath.Join(spaceDir, ".gitignore")
	if _, err := os.Stat(gitignore); err != nil {
		if err := os.WriteFile(gitignore, []byte(".env\noutput/\n"), 0644); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
	}
	return nil
}

// GitCommit stages all changes and commits with the given message.
// 不使用 --allow-empty：如果 git add -A 失败或没有实际变更，不会产生空 commit。
// "nothing to commit" 不视为错误（幂等场景下可能无变更）。
func GitCommit(spaceDir, message string) error {
	if err := exec.Command("git", "-C", spaceDir, "add", "-A").Run(); err != nil {
		return fmt.Errorf("git add -A: %w", err)
	}
	out, err := exec.Command("git", "-C", spaceDir, "commit", "-m", message).CombinedOutput()
	if err != nil {
		// "nothing to commit" 不是错误——幂等场景下可能无实际变更
		if strings.Contains(string(out), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}

// GitLog returns recent commit history as structured entries.
type GitLogEntry struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

func GitLog(spaceDir string, limit int) []GitLogEntry {
	if limit <= 0 {
		limit = 50
	}
	// Format: hash|date|message
	cmd := exec.Command("git", "-C", spaceDir, "log",
		"--pretty=format:%h|%aI|%s",
		"-n", strings.TrimSpace(itoa(limit)))
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var entries []GitLogEntry
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		entries = append(entries, GitLogEntry{
			Hash:    parts[0],
			Date:    parts[1],
			Message: parts[2],
		})
	}
	return entries
}

// GitRevert reverts a specific commit.
// Deprecated: 对于本地知识库，GitRestoreHash 更好（瞬时恢复，无需重编译）。
func GitRevert(spaceDir, hash string) (string, error) {
	cmd := exec.Command("git", "-C", spaceDir, "revert", "--no-edit", hash)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// GitRestoreHash 用 git checkout <hash> -- . 恢复到指定 commit 的完整快照，
// 然后创建一个新 commit 保留历史。
//
// 对比 git revert：
//   - revert：只反转 diff，.openkb/ 索引不随 raw 一起恢复 → 需要重编译
//   - checkout -- .：恢复所有文件（raw + .openkb/）到那个 commit 的状态 → 瞬时完成
//
// 因为 .openkb/ 不在 .gitignore 中，每次 commit 都保存了完整索引快照，
// 所以 checkout 能同时恢复 raw 文件和知识库索引，无需 recompile。
func GitRestoreHash(spaceDir, hash string) error {
	// 1) checkout 指定 hash 的所有文件到工作区
	if out, err := exec.Command("git", "-C", spaceDir, "checkout", hash, "--", ".").CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s -- .: %w\n%s", hash, err, out)
	}
	return nil
}

// GitShowFiles returns files changed in a specific commit.
func GitShowFiles(spaceDir, hash string) []string {
	cmd := exec.Command("git", "-C", spaceDir, "diff-tree", "--no-commit-id", "--name-only", "-r", hash)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		if l != "" {
			files = append(files, l)
		}
	}
	return files
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
