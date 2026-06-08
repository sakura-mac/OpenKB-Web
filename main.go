package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"time"

	"okb-web/internal/config"
	"okb-web/internal/handler"
	"okb-web/internal/okb"
	"okb-web/internal/watch"

	"github.com/gin-gonic/gin"
)

//go:embed web/dist/*
var webFS embed.FS

func main() {
	config.Init()

	// 首次启动：异步装 OpenKB（拉 uv standalone + uv tool install）。
	// HTTP 服务立即启动，前端通过 /api/bootstrap/status 看进度，没就绪时显示遮罩。
	// 失败也不致命：用户能改设置后调 /api/bootstrap/retry 重跑。
	okb.BootstrapAsync()

	// 文件 watcher：监听每个 space 的 raw/，自动 spawn openkb add/remove。
	// handler 包通过 SetWatchManager 拿到引用，CreateSpace/DeleteSpace 时挂载/卸载。
	wm := watch.NewManager()
	handler.SetWatchManager(wm)
	bootstrapWatchers(wm)
	defer wm.Close()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// API
	api := r.Group("/api")
	{
		api.GET("/spaces", handler.ListSpaces)
		api.GET("/space/:name", handler.SpaceDetail)
		api.GET("/wiki/:space/:category/:page", handler.WikiPage)
		api.POST("/spaces/create", handler.CreateSpace)
		api.GET("/space-status/:name", handler.SpaceStatus)
		api.POST("/spaces/delete", handler.DeleteSpace)
		api.POST("/query", handler.Query)
		api.POST("/add", handler.AddDoc)
		api.POST("/upload/:space", handler.UploadDoc)
		api.GET("/task/:id", handler.GetTaskStatus)
		api.POST("/remove", handler.RemoveDoc)
		api.POST("/browse", handler.Browse)
		api.GET("/history/:space", handler.History)
		api.POST("/revert", handler.Revert)
		api.POST("/recompile/:space", handler.Recompile)
		api.GET("/lint/:space", handler.Lint)
		api.GET("/status/:space", handler.Status)
		api.GET("/graph/:space", handler.Graph)
		api.GET("/locale", handler.DetectLocale)
		api.GET("/suggestions/:space", handler.Suggestions)

		// 设置（LLM API key / base url / model / spaces 路径）
		// 持久化在 <OKB_HOME>/config.json
		api.GET("/settings", handler.GetSettings)
		api.POST("/settings", handler.UpdateSettings)
		api.POST("/settings/check", handler.CheckSettings)

		// Bootstrap 状态：前端轮询初始化进度（uv 下载 / OpenKB 安装 / 资源释放）
		api.GET("/bootstrap/status", handler.GetBootstrapStatus)
		api.POST("/bootstrap/retry", handler.RetryBootstrap)

		// Deck（HTML 幻灯片导出）
		api.POST("/deck", handler.CreateDeck)
		api.GET("/decks/:space", handler.ListDecks)
		api.DELETE("/deck/:space/:name", handler.DeleteDeck)
		api.GET("/deck/:space/:name", handler.ServeDeck) // ?download=1 触发下载

		// Chat（多轮，复用 OpenKB 的 chat session 持久化）
		api.GET("/chat/sessions/:space", handler.ChatListSessions)
		api.GET("/chat/session/:space/:sid", handler.ChatLoadSession)
		api.DELETE("/chat/session/:space/:sid", handler.ChatDeleteSession)
		api.POST("/chat/send", handler.ChatSend)
		api.POST("/chat/stream", handler.ChatStream)
	}

	// SPA frontend (embedded)
	r.NoRoute(handler.ServeFrontend(webFS))

	// 定时清理过期的异步任务，避免内存泄漏
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			okb.CleanOldTasks()
		}
	}()

	addr := "0.0.0.0:" + config.C.Port
	log.Printf("🚀 OKB Web 启动: http://localhost:%s", config.C.Port)
	log.Printf("📁 空间目录: %s", config.C.SpacesRoot)
	r.Run(addr)
}

// bootstrapWatchers 进程启动时给每个已存在的 space 挂上 raw/ watcher。
// 判定 space：SpacesRoot 下的目录且包含 .openkb（与 ListSpaces 同条件）。
func bootstrapWatchers(wm *watch.Manager) {
	root := config.C.SpacesRoot
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			// 跟随 symlink
			info, err := os.Stat(filepath.Join(root, e.Name()))
			if err != nil || !info.IsDir() {
				continue
			}
		}
		spaceDir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(spaceDir, ".openkb")); err != nil {
			continue
		}
		if err := wm.EnsureSpace(spaceDir); err != nil {
			log.Printf("⚠️  watch 启动失败 [%s]: %v", spaceDir, err)
		}
	}
}
