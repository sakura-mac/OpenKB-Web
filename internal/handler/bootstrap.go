package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"okb-web/internal/okb"
)

// GetBootstrapStatus 返回当前 bootstrap 状态。前端轮询用：
//   - 启动后立即 GET，看到非 "ready" 就显示初始化遮罩
//   - 持续 polling 直到 ready 或 failed
//   - failed 时显示错误 + "重试"按钮，重试调 POST /api/bootstrap/retry
func GetBootstrapStatus(c *gin.Context) {
	c.JSON(http.StatusOK, okb.GetStatus())
}

// RetryBootstrap 让用户改完设置（如 OKB Spec / 网络）后能重新触发安装。
// 内部走 BootstrapAsync，立即返回 status；前端继续轮询。
func RetryBootstrap(c *gin.Context) {
	okb.BootstrapAsync()
	c.JSON(http.StatusAccepted, okb.GetStatus())
}
