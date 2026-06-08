package okb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitInit initializes a git repo in the space directory (idempotent).
func GitInit(spaceDir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = spaceDir
	cmd.Run()

	// Set user for commits
	exec.Command("git", "-C", spaceDir, "config", "user.email", "okb-web@local").Run()
	exec.Command("git", "-C", spaceDir, "config", "user.name", "OKB Web").Run()

	// Create .gitignore if not exists（直接写文件，避免 echo 不解析 \n 导致 .env 被纳入版本库）
	// output/ 是 deck/skill 等命令的产物目录，不应进版本库
	gitignore := filepath.Join(spaceDir, ".gitignore")
	if _, err := os.Stat(gitignore); err != nil {
		os.WriteFile(gitignore, []byte(".env\noutput/\n"), 0644)
	}
}

// GitCommit stages all changes and commits with the given message.
func GitCommit(spaceDir, message string) error {
	exec.Command("git", "-C", spaceDir, "add", "-A").Run()
	cmd := exec.Command("git", "-C", spaceDir, "commit", "-m", message, "--allow-empty")
	return cmd.Run()
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
func GitRevert(spaceDir, hash string) (string, error) {
	cmd := exec.Command("git", "-C", spaceDir, "revert", "--no-edit", hash)
	out, err := cmd.CombinedOutput()
	return string(out), err
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
