package handler

import (
	"bufio"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Suggestions 返回 4 条「让用户开始问答」的推荐问题。
//
// 思路：
//   1. 从 wiki/concepts 和 wiki/entities 各抽几个 slug，读 H1 标题作为人类可读名字
//   2. 套用问题模板（按 ?lang=zh-CN / en 切换）
//   3. 4 条里至少包含 2 类问题：解释概念、对比关系；剩余按可用素材填充
//
// 没素材时返回通用兜底问题（"这个知识库里有什么"等）。
//
// 路由：GET /api/suggestions/:space?lang=zh-CN
func Suggestions(c *gin.Context) {
	space := c.Param("space")
	spaceDir, ok := resolveSpace(c, space)
	if !ok {
		return
	}
	lang := c.Query("lang")
	if lang != "en" {
		lang = "zh-CN" // 默认中文
	}

	wikiDir := filepath.Join(spaceDir, "wiki")
	concepts := pickTitled(filepath.Join(wikiDir, "concepts"), 6)
	entities := pickTitled(filepath.Join(wikiDir, "entities"), 6)
	summaries := pickTitled(filepath.Join(wikiDir, "summaries"), 4)

	suggestions := buildSuggestions(lang, concepts, entities, summaries)

	c.JSON(200, gin.H{"suggestions": suggestions})
}

// titled 表示一个有 slug 和 H1 标题的 wiki 页面。
type titled struct {
	Slug  string
	Title string
}

// pickTitled 从目录里随机选最多 n 篇 .md 文档，并提取它们的 H1 标题。
// H1 找不到就退化为 slug 本身（仍可读）。
func pickTitled(dir string, n int) []titled {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var slugs []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		slugs = append(slugs, strings.TrimSuffix(name, ".md"))
	}
	if len(slugs) == 0 {
		return nil
	}
	rand.Shuffle(len(slugs), func(i, j int) { slugs[i], slugs[j] = slugs[j], slugs[i] })
	if n > len(slugs) {
		n = len(slugs)
	}
	out := make([]titled, 0, n)
	for _, slug := range slugs[:n] {
		title := readH1(filepath.Join(dir, slug+".md"))
		if title == "" {
			title = humanize(slug)
		}
		out = append(out, titled{Slug: slug, Title: title})
	}
	return out
}

// readH1 读取 markdown 文件的第一行 `# Title`，找不到返回空串。
// 仅扫描前 32 行，性能可控。
func readH1(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; i < 32 && sc.Scan(); i++ {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(line[2:])
		}
	}
	return ""
}

// humanize 把 kebab-case slug 还原成自然语句（仅用作 H1 读不到时的兜底）。
//   "self-healing-selector" → "Self Healing Selector"
func humanize(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// buildSuggestions 按可用素材组装 4 条推荐问题。
// 顺序：1 概念解释 + 1 实体说明 + 1 对比 + 1 概览 (按可用性 fallback)。
func buildSuggestions(lang string, concepts, entities, summaries []titled) []string {
	var out []string

	// Q1: 解释一个概念
	if len(concepts) > 0 {
		out = append(out, tplExplain(lang, concepts[0].Title))
	}
	// Q2: 介绍一个实体
	if len(entities) > 0 {
		out = append(out, tplWho(lang, entities[0].Title))
	}
	// Q3: 对比两个概念（or 实体）
	if len(concepts) >= 2 {
		out = append(out, tplCompare(lang, concepts[0].Title, concepts[1].Title))
	} else if len(entities) >= 2 {
		out = append(out, tplCompare(lang, entities[0].Title, entities[1].Title))
	}
	// Q4: 摘要某文档 / 问知识库主题
	if len(summaries) > 0 {
		out = append(out, tplSummarize(lang, summaries[0].Title))
	} else if len(concepts) >= 3 {
		out = append(out, tplExplain(lang, concepts[2].Title))
	}

	// 兜底凑齐 4 条
	for len(out) < 4 {
		out = append(out, tplOverview(lang, len(out)))
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func tplExplain(lang, name string) string {
	if lang == "en" {
		return "Explain \"" + name + "\" and why it matters."
	}
	return "「" + name + "」是什么？为什么重要？"
}

func tplWho(lang, name string) string {
	if lang == "en" {
		return "Who/what is " + name + "? Give me a short bio."
	}
	return name + " 是谁/什么？请简要介绍。"
}

func tplCompare(lang, a, b string) string {
	if lang == "en" {
		return "Compare \"" + a + "\" and \"" + b + "\"."
	}
	return "对比一下「" + a + "」和「" + b + "」。"
}

func tplSummarize(lang, name string) string {
	if lang == "en" {
		return "Summarize \"" + name + "\" in three bullet points."
	}
	return "用三点概括「" + name + "」。"
}

// tplOverview 是兜底问题，按 idx 取不同模板避免重复。
func tplOverview(lang string, idx int) string {
	if lang == "en" {
		switch idx % 4 {
		case 0:
			return "What is this knowledge base about?"
		case 1:
			return "List the most important concepts here."
		case 2:
			return "Who are the key people mentioned?"
		default:
			return "What should I read first?"
		}
	}
	switch idx % 4 {
	case 0:
		return "这个知识库讲的是什么？"
	case 1:
		return "列出这里最重要的几个概念。"
	case 2:
		return "里面提到的关键人物有哪些？"
	default:
		return "我应该先读哪一篇？"
	}
}
