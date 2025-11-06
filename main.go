package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"pic/config"
	"pic/handlers"
	"pic/middleware"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 初始化配置
	config.InitDB()

	// 创建Gin路由
	r := gin.Default()

	// 允许跨域
	r.Use(middleware.CORS())

	// 公开路由
	public := r.Group("/api")
	{
		public.POST("/auth/login", handlers.Login)
		public.POST("/auth/register", handlers.Register)
	}

	// 公开路由（无需认证）
	r.GET("/api/gallery/:slug", handlers.GetPublicGallery)

	// 需要认证的路由
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	{
		// GitHub相关
		protected.GET("/github/repos", handlers.GetRepositories)
		protected.POST("/github/verify-token", handlers.VerifyGitHubToken)

		// 配置管理
		protected.POST("/config", handlers.SaveConfig)
		protected.GET("/config", handlers.GetConfig)
		protected.GET("/gallery/check-slug", handlers.CheckGallerySlug)

		// 图片上传
		protected.POST("/upload", handlers.UploadImage)
		protected.GET("/images", handlers.GetImages)
		protected.DELETE("/images/:id", handlers.DeleteImage)
	}

	// 静态文件服务（前端）
	r.Static("/assets", "./frontend/dist/assets")
	r.StaticFile("/favicon.svg", "./frontend/dist/favicon.svg")

	// SPA路由支持：所有非API请求都返回index.html
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// 排除API请求
		if strings.HasPrefix(path, "/api") {
			c.JSON(404, gin.H{"error": "API路由不存在"})
		} else {
			c.File("./frontend/dist/index.html")
		}
	})

	log.Println("🚀 服务器启动在 http://localhost:9090")

	// 创建HTTP服务器
	srv := &http.Server{
		Addr:           ":9090",
		Handler:        r,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   60 * time.Second, // 增加到60秒以支持大文件上传
		MaxHeaderBytes: 1 << 20,          // 1MB
	}

	// 在goroutine中启动服务器
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("服务器启动失败:", err)
		}
	}()

	// 等待中断信号以优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
	log.Println("🔄 正在关闭服务器...")

	// 设置30秒超时的context用于优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("服务器关闭超时或出错: %v", err)
	}

	// 关闭数据库连接
	if config.DB != nil {
		sqlDB, err := config.DB.DB()
		if err != nil {
			log.Printf("获取底层数据库连接失败: %v", err)
		} else {
			if err := sqlDB.Close(); err != nil {
				log.Printf("关闭数据库连接失败: %v", err)
			} else {
				log.Println("✅ 数据库连接已关闭")
			}
		}
	}

	log.Println("✅ 服务器已优雅退出")
}
