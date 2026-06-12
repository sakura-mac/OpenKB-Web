package model

// SpaceInfo is returned in the space list.
type SpaceInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind,omitempty"` // kb | code；空兼容旧前端，等价 kb
	Docs     int    `json:"docs"`
	Concepts int    `json:"concepts"`
	Files    int    `json:"files,omitempty"`
	Indexed  bool   `json:"indexed,omitempty"`
}

// DocInfo represents a document in raw/.
// DisplayName 可选：URL 抓取的文档展示用「人类可读标题」（如 "Attention Is All You Need"）。
// 文件本身的物理 Name 由 OpenKB 决定（一般是 host+path 的 slug），不可改。
type DocInfo struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"display_name,omitempty"`
	SourceURL   string  `json:"source_url,omitempty"`
	Size        int64   `json:"size"`
	Modified    float64 `json:"modified"`
}

// SpaceDetail is returned for a single space.
//
// Titles 是 raw-slug（去扩展名）→ 人类可读标题 的映射，
// 让前端 summaries/concepts 这类「只有 slug」的列表也能渲染漂亮的标题。
type SpaceDetail struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Docs      []DocInfo `json:"docs"`
	Summaries []string  `json:"summaries"`
	Concepts  []string  `json:"concepts"`
	Entities  []string  `json:"entities"`
	// Explorations: chat agent 在对话里临时综合多个 wiki 写的分析笔记
	// （对比表 / 专题深挖）。OpenKB 用 wiki/explorations/ 目录承载，
	// 让前端能列出这一类，作为 wiki 的第 4 个 tab。
	Explorations []string          `json:"explorations,omitempty"`
	Titles       map[string]string `json:"titles,omitempty"`
}

// Request types

type CreateSpaceReq struct {
	Name string `json:"name" binding:"required"`
	Path string `json:"path"`
}

type CodeSpaceInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Indexed    bool   `json:"indexed"`
	Files      int    `json:"files"`
	Codegraph  string `json:"codegraph,omitempty"`
	ModifiedAt int64  `json:"modified_at,omitempty"`
}

type CreateCodeSpaceReq struct {
	Name string `json:"name" binding:"required"`
	Path string `json:"path" binding:"required"`
}

type CodeQueryReq struct {
	Space    string `json:"space" binding:"required"`
	Question string `json:"question" binding:"required"`
}

type CodeSyncReq struct {
	Space string `json:"space" binding:"required"`
}

type DeleteSpaceReq struct {
	Name string `json:"name" binding:"required"`
}

type QueryReq struct {
	Space    string `json:"space" binding:"required"`
	Question string `json:"question" binding:"required"`
}

type AddDocReq struct {
	Space string `json:"space" binding:"required"`
	Path  string `json:"path" binding:"required"`
}

type RemoveDocReq struct {
	Space string `json:"space" binding:"required"`
	Doc   string `json:"doc" binding:"required"`
}
