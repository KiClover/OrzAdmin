package middleware

import (
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/hertz-contrib/cors"
)

func NewCORSMw() app.HandlerFunc {
	corsMw := cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:3002",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"SecretKey",
			"secretkey",
			"X-Requested-With",
		},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: false,
		AllowOriginFunc: func(origin string) bool {
			// 允许本地开发环境任意端口（如 http://localhost:xxxx / http://127.0.0.1:xxxx）
			if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
				return true
			}
			// 生产环境请在此处按需放行你的前端域名（示例： https://example.com ）
			return false
		},
		MaxAge: 12 * time.Hour,
	})
	return corsMw
}
