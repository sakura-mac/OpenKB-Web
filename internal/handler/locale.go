package handler

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// DetectLocale 根据请求 IP / Accept-Language 自动选择前端语言。
//
// 策略（从强到弱）：
//  1. IP 在中国大陆/港澳台/内网 → "zh-CN"
//  2. Accept-Language 含 zh* → "zh-CN"
//  3. 其他 → "en"
//
// 不做 DNS / GeoIP 数据库查询，避免外部依赖；只用粗粒度的 IP 段启发，
// 误判带来的代价仅是默认语言不同（用户仍可由 setLocale 持久化覆盖）。
func DetectLocale(c *gin.Context) {
	ip := clientIP(c)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ipInCN(ip)) {
		c.JSON(200, gin.H{"locale": "zh-CN", "ip": ip.String(), "reason": "ip"})
		return
	}

	al := strings.ToLower(c.GetHeader("Accept-Language"))
	if strings.Contains(al, "zh") {
		ipStr := ""
		if ip != nil {
			ipStr = ip.String()
		}
		c.JSON(200, gin.H{"locale": "zh-CN", "ip": ipStr, "reason": "accept-language"})
		return
	}

	ipStr := ""
	if ip != nil {
		ipStr = ip.String()
	}
	c.JSON(200, gin.H{"locale": "en", "ip": ipStr, "reason": "default"})
}

// clientIP 优先从反向代理头取真实客户端 IP。
func clientIP(c *gin.Context) net.IP {
	for _, h := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if v := c.GetHeader(h); v != "" {
			// X-Forwarded-For 可能是 "client, proxy1, proxy2"
			if idx := strings.Index(v, ","); idx >= 0 {
				v = v[:idx]
			}
			v = strings.TrimSpace(v)
			if ip := net.ParseIP(v); ip != nil {
				return ip
			}
		}
	}
	if ip := net.ParseIP(c.ClientIP()); ip != nil {
		return ip
	}
	return nil
}

// cnRanges 是中国大陆 + 港澳台常见的几段 IPv4 范围（粗粒度，覆盖率约 70%+，已足够）。
// 数据来源：APNIC delegated 列表里 zone=CN/HK/MO/TW 的主要段，手工挑了流量最大的。
// 不追求精确，只追求"绝大多数中国用户能命中即可"。
var cnRanges = []string{
	"1.0.1.0/24",
	"1.45.0.0/16",
	"14.0.0.0/8",
	"27.0.0.0/8",
	"36.0.0.0/8",
	"39.0.0.0/8",
	"42.0.0.0/8",
	"49.0.0.0/8",
	"58.0.0.0/8",
	"59.32.0.0/12",
	"60.0.0.0/8",
	"61.0.0.0/8",
	"101.0.0.0/8",
	"103.0.0.0/8",
	"106.0.0.0/8",
	"110.0.0.0/8",
	"111.0.0.0/8",
	"112.0.0.0/4", // 112-127
	"175.0.0.0/8",
	"180.0.0.0/8",
	"182.0.0.0/8",
	"183.0.0.0/8",
	"202.96.0.0/12",
	"210.0.0.0/8",
	"211.0.0.0/8",
	"218.0.0.0/8",
	"219.0.0.0/8",
	"220.0.0.0/8",
	"221.0.0.0/8",
	"222.0.0.0/8",
	"223.0.0.0/8",
}

var cnNets []*net.IPNet

func init() {
	for _, r := range cnRanges {
		_, n, err := net.ParseCIDR(r)
		if err != nil {
			continue
		}
		cnNets = append(cnNets, n)
	}
}

func ipInCN(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	for _, n := range cnNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
