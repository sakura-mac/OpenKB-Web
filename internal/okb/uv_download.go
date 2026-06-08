package okb

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"okb-web/internal/config"
)

// uvFetchVersion 锁定的 uv 版本——不用 latest 防止 GitHub 限流时拉到坏版本。
// 升级 uv 时改这一处。
const uvFetchVersion = "0.5.18"

// downloadUv 下载 uv standalone 二进制到 <OKBHome>/runtime/bin/uv（或 .exe）。
//
// 返回安装好的 uv 路径。
//
// 流程：
//  1. 按 GOOS/GOARCH 决定 target triple
//  2. 拉 https://github.com/astral-sh/uv/releases/download/<ver>/uv-<triple>.tar.gz（Win 是 .zip）
//  3. 解压取里面的 uv 可执行文件，写到 runtime/bin/uv
//  4. chmod +x（Win 不需要）
//
// 进度通过 updateStatus(PhaseDownloadUv, ...) 推给前端轮询。
func downloadUv() (string, error) {
	triple, archive, err := uvTargetTriple()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://github.com/astral-sh/uv/releases/download/%s/uv-%s.%s",
		uvFetchVersion, triple, archive,
	)
	updateStatus(PhaseDownloadUv, 12, fmt.Sprintf("下载 uv（%s）...", triple))

	binDir := filepath.Join(config.C.RuntimeDir, "bin")
	_ = os.MkdirAll(binDir, 0o755)

	dstName := "uv"
	if runtime.GOOS == "windows" {
		dstName = "uv.exe"
	}
	dst := filepath.Join(binDir, dstName)

	// 下载
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("下载 uv 失败：%w（url=%s）", err, url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载 uv 失败：HTTP %d（url=%s）", resp.StatusCode, url)
	}

	updateStatus(PhaseDownloadUv, 25, "解压 uv...")

	if archive == "tar.gz" {
		if err := extractTarGzPickFile(resp.Body, "uv", dst); err != nil {
			return "", fmt.Errorf("解压 uv tar.gz：%w", err)
		}
	} else {
		// .zip：先全量缓存到内存（uv zip 体积 < 20MB，可接受）
		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		if err := extractZipPickFile(buf, "uv.exe", dst); err != nil {
			return "", fmt.Errorf("解压 uv zip：%w", err)
		}
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(dst, 0o755)
	}
	updateStatus(PhaseDownloadUv, 35, "uv 安装完成")
	return dst, nil
}

// uvTargetTriple 把 Go 的 GOOS/GOARCH 翻译成 uv release 的 target triple + 归档格式。
func uvTargetTriple() (triple, archive string, err error) {
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "x86_64-unknown-linux-gnu", "tar.gz", nil
		case "arm64":
			return "aarch64-unknown-linux-gnu", "tar.gz", nil
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "x86_64-apple-darwin", "tar.gz", nil
		case "arm64":
			return "aarch64-apple-darwin", "tar.gz", nil
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "x86_64-pc-windows-msvc", "zip", nil
		}
	}
	return "", "", fmt.Errorf("不支持的平台 %s/%s", runtime.GOOS, runtime.GOARCH)
}

// extractTarGzPickFile 从 .tar.gz 流里挑出名字为 want（basename）的文件，写到 dst。
//
// uv 的 tar.gz 顶层是个目录 uv-<triple>/，里面有 uv 二进制。
// 只匹配 basename（不关心顶层目录是 uv-x86_64-... 还是别的）。
func extractTarGzPickFile(r io.Reader, want, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("归档中没找到 %s", want)
		}
		if err != nil {
			return err
		}
		if filepath.Base(h.Name) != want {
			continue
		}
		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		f.Close()
		return err
	}
}

// extractZipPickFile 从 .zip 字节里挑出 want 文件写到 dst。
func extractZipPickFile(zipBytes []byte, want, dst string) error {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		if !strings.EqualFold(filepath.Base(f.Name), want) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, rc)
		return err
	}
	return fmt.Errorf("zip 中没找到 %s", want)
}
