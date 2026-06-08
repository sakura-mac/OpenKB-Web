package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// urlTitleCache 把每个 URL 抓取的 <title> 持久化到
//   <space>/.openkb/url-titles.json
// 形如：
//   {
//     "<raw-slug-without-ext>": {
//       "title": "Attention Is All You Need",
//       "url":   "https://arxiv.org/...",
//       "ts":    1718000000
//     }
//   }
//
// 写时持锁，读时不持锁（前端用映射只是用于显示，偶发竞态可接受）。

type urlTitleEntry struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	TS    int64  `json:"ts"`
}

var urlTitleMu sync.Mutex

func urlTitlesPath(spaceDir string) string {
	return filepath.Join(spaceDir, ".openkb", "url-titles.json")
}

// loadURLTitles 读 url-titles.json；不存在或解析失败都返回空 map。
func loadURLTitles(spaceDir string) map[string]urlTitleEntry {
	out := map[string]urlTitleEntry{}
	data, err := os.ReadFile(urlTitlesPath(spaceDir))
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

// saveURLTitle 把 slug→title/url 写入 .openkb/url-titles.json。
// slug 由调用方传入（通常是 raw 文件名去扩展名）。
func saveURLTitle(spaceDir, slug, title, srcURL string) {
	if slug == "" || title == "" {
		return
	}
	urlTitleMu.Lock()
	defer urlTitleMu.Unlock()

	titles := loadURLTitles(spaceDir)
	titles[slug] = urlTitleEntry{Title: title, URL: srcURL, TS: time.Now().Unix()}

	if err := os.MkdirAll(filepath.Dir(urlTitlesPath(spaceDir)), 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(titles, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(urlTitlesPath(spaceDir), data, 0644)
}

// fetchPageTitle 抓取 URL 取 <title>。失败时返回空串，调用方应自行降级。
//
// 限制：
//   - 总超时 8s（url 抓取后端流程整体走异步 task，这里不该再阻塞太久）
//   - 只读前 64KB（title 一般在前面几百字节里）
//   - 只支持 text/html；PDF/其他类型直接放弃
func fetchPageTitle(ctx context.Context, raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}

	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(cctx, http.MethodGet, raw, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (compatible; OKB-Web/1.0; +https://github.com/khalilfchen/okb-web)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	client := &http.Client{Timeout: 9 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(strings.ToLower(ct), "html") {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return ""
	}

	return parseTitle(string(body))
}

// titleRe 匹配 <title>...</title>，多行，忽略大小写，最长 500 字符（防 DoS）。
var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.{1,500}?)</title>`)
var ogTitleRe = regexp.MustCompile(`(?is)<meta[^>]+property\s*=\s*["']og:title["'][^>]*content\s*=\s*["']([^"']{1,500})["']`)
var twTitleRe = regexp.MustCompile(`(?is)<meta[^>]+name\s*=\s*["']twitter:title["'][^>]*content\s*=\s*["']([^"']{1,500})["']`)

// parseTitle 优先级：og:title > twitter:title > <title>。
// 这三者比 <title> 更接近"展示标题"（<title> 经常带站点名后缀）。
func parseTitle(html string) string {
	for _, re := range []*regexp.Regexp{ogTitleRe, twTitleRe, titleRe} {
		if m := re.FindStringSubmatch(html); len(m) > 1 {
			t := decodeHTMLEntities(m[1])
			t = strings.TrimSpace(strings.Join(strings.Fields(t), " "))
			if t != "" {
				return t
			}
		}
	}
	return ""
}

var entityMap = map[string]string{
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": "\"",
	"&apos;": "'",
	"&#39;":  "'",
	"&nbsp;": " ",
	"&mdash;": "—",
	"&ndash;": "–",
	"&hellip;": "…",
}

func decodeHTMLEntities(s string) string {
	for k, v := range entityMap {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}
