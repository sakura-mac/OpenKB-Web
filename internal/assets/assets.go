// Package assets 通过 go:embed 把 OpenKB 跑起来必备的资产（chat_helper.py、deck/critic skills）
// 打进 okb-web 二进制，运行时按需释放到磁盘上的固定位置。
//
// 这样用户拿到的二进制是真·自包含：除了 uv（外部依赖，~10MB）之外，不需要 git clone 任何东西。
package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed chat_helper.py
var chatHelperPy []byte

//go:embed all:skills
var skillsFS embed.FS

// ChatHelperPath 返回 chat_helper.py 释放后的绝对路径（首次调用时写盘）。
func ChatHelperPath(cacheDir string) (string, error) {
	dst := filepath.Join(cacheDir, "chat_helper.py")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	// 始终覆盖：保证升级 okb-web 后 helper 也跟着升级
	if err := os.WriteFile(dst, chatHelperPy, 0o644); err != nil {
		return "", fmt.Errorf("write chat_helper.py: %w", err)
	}
	return dst, nil
}

// SkillsDir 把 embed 的 skills/ 释放到 <cacheDir>/skills/ 并返回其绝对路径。
// 每次启动都重写（skills 体量小、且要随 okb-web 升级）。
func SkillsDir(cacheDir string) (string, error) {
	dst := filepath.Join(cacheDir, "skills")
	if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}

	err := fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// path 形如 "skills/openkb-deck-editorial/SKILL.md"
		rel, _ := filepath.Rel("skills", path)
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := skillsFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("extract skills: %w", err)
	}
	return dst, nil
}
