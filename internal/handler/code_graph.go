package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"okb-web/internal/codegraph"
)

// ============================================================
// 代码图谱：按需实时构建（非预设）
//
// 文档知识库的图谱是编译期建好的；代码图谱不一样——它太大（48k 文件），
// 不可能整图渲染。做法是「以符号为中心、按需展开」：
//   - 给一个符号 → 查它的 callers（谁调它）+ callees（它调谁）= 1 跳邻居子图
//   - 前端点击任一节点 → 再拉那个节点的邻居，动态长出图（实时渲染）
//   - 点击节点 → 拉它的源码片段展示（「点进有东西看」）
// ============================================================

type cgNodeRaw struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	FilePath      string `json:"filePath"`
	StartLine     int    `json:"startLine"`
	EndLine       int    `json:"endLine"`
	Docstring     string `json:"docstring"`
}

type cgQueryItem struct {
	Node  cgNodeRaw `json:"node"`
	Score float64   `json:"score"`
}

type cgRelItem struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	FilePath  string `json:"filePath"`
	StartLine int    `json:"startLine"`
}

// 图谱节点 / 边（发给前端 cytoscape）
type graphNodeOut struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	IsCenter bool   `json:"is_center,omitempty"`
}

type graphEdgeOut struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // caller | callee
}

const maxNeighbors = 16

// CodeGraphNeighbors GET /api/code/graph/:space?symbol=X
// 以 symbol 为中心，返回 1 跳邻居子图（callers + callees）。
func CodeGraphNeighbors(c *gin.Context) {
	cs, err := readCodeSpace(c.Param("space"))
	if err != nil {
		c.JSON(404, gin.H{"error": "代码空间不存在"})
		return
	}
	name := strings.TrimSpace(c.Query("symbol"))
	if name == "" {
		c.JSON(400, gin.H{"error": "缺少 symbol"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 25*time.Second)
	defer cancel()

	nodes := map[string]*graphNodeOut{}
	var edges []graphEdgeOut
	addNode := func(n graphNodeOut) {
		if n.ID == "" {
			return
		}
		if ex, ok := nodes[n.ID]; ok {
			if n.IsCenter {
				ex.IsCenter = true
			}
			return
		}
		cp := n
		nodes[n.ID] = &cp
	}

	// 中心节点：query 拿自身元数据
	center := graphNodeOut{ID: name, Label: name, IsCenter: true}
	if ok, out, _ := codegraph.Run(ctx, cs.Path, "query", name, "-j", "-l", "1"); ok {
		var items []cgQueryItem
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &items) == nil && len(items) > 0 {
			nd := items[0].Node
			center.Kind = nd.Kind
			center.File = nd.FilePath
			center.Line = nd.StartLine
			if nd.Name != "" {
				center.Label = nd.Name
			}
		}
	}
	addNode(center)

	// callees：center → 它调用的
	if ok, out, _ := codegraph.Run(ctx, cs.Path, "callees", name, "-j"); ok {
		var res struct {
			Callees []cgRelItem `json:"callees"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &res) == nil {
			for i, r := range res.Callees {
				if i >= maxNeighbors {
					break
				}
				addNode(graphNodeOut{ID: r.Name, Label: r.Name, Kind: r.Kind, File: r.FilePath, Line: r.StartLine})
				edges = append(edges, graphEdgeOut{Source: name, Target: r.Name, Type: "callee"})
			}
		}
	}

	// callers：调用 center 的 → center
	if ok, out, _ := codegraph.Run(ctx, cs.Path, "callers", name, "-j"); ok {
		var res struct {
			Callers []cgRelItem `json:"callers"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &res) == nil {
			for i, r := range res.Callers {
				if i >= maxNeighbors {
					break
				}
				addNode(graphNodeOut{ID: r.Name, Label: r.Name, Kind: r.Kind, File: r.FilePath, Line: r.StartLine})
				edges = append(edges, graphEdgeOut{Source: r.Name, Target: name, Type: "caller"})
			}
		}
	}

	out := make([]graphNodeOut, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, *n)
	}
	c.JSON(200, gin.H{"nodes": out, "edges": edges})
}

// CodeSymbolSource GET /api/code/symbol/:space?name=X
// 返回某符号的源码片段 + 元数据（点击图谱节点时展示）。
func CodeSymbolSource(c *gin.Context) {
	cs, err := readCodeSpace(c.Param("space"))
	if err != nil {
		c.JSON(404, gin.H{"error": "代码空间不存在"})
		return
	}
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		c.JSON(400, gin.H{"error": "缺少 name"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	ok, out, _ := codegraph.Run(ctx, cs.Path, "query", name, "-j", "-l", "1")
	if !ok {
		c.JSON(200, gin.H{"found": false})
		return
	}
	var items []cgQueryItem
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &items) != nil || len(items) == 0 {
		c.JSON(200, gin.H{"found": false})
		return
	}
	nd := items[0].Node
	end := nd.EndLine
	if end < nd.StartLine {
		end = nd.StartLine + 80
	}
	code := readSource(cs.Path, nd.FilePath, nd.StartLine, end)
	c.JSON(200, gin.H{
		"found":      true,
		"name":       nd.Name,
		"kind":       nd.Kind,
		"qualified":  nd.QualifiedName,
		"file":       nd.FilePath,
		"start_line": nd.StartLine,
		"end_line":   nd.EndLine,
		"docstring":  nd.Docstring,
		"code":       code,
	})
}
