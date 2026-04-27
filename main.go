package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"seo-system/internal/api"
	"seo-system/internal/db"
	"seo-system/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()
	middleware.MustInitJWT()

	if os.Getenv("APP_PASSWORD") == "" {
		log.Fatal("APP_PASSWORD environment variable must be set (single-user login password).")
	}

	db.Init()
	_ = api.DefaultServer()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[boot] WarmDataCache panic: %v", r)
			}
		}()
		api.WarmDataCache()
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[boot] BackfillAuditFindings panic: %v", r)
			}
		}()
		api.BackfillAuditFindings()
	}()

	r := gin.New()
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		path := param.Path
		if strings.Contains(path, "/oauth/") {
			if idx := strings.Index(path, "?"); idx >= 0 {
				path = path[:idx] + "?[REDACTED]"
			}
		}
		return fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %-7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.StatusCode,
			param.Latency,
			param.ClientIP,
			param.Method,
			path,
		)
	}))
	r.Use(gin.Recovery())

	trusted := []string{"127.0.0.1", "::1"}
	if raw := os.Getenv("TRUSTED_PROXIES"); raw != "" {
		trusted = trusted[:0]
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				trusted = append(trusted, p)
			}
		}
	}
	if err := r.SetTrustedProxies(trusted); err != nil {
		log.Fatalf("SetTrustedProxies 失败: %v", err)
	}

	r.Use(middleware.CORS())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.RecoverLogger())
	api.RegisterRoutes(r, middleware.AuthRequired(), middleware.CSRFProtect())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	api.StartOAuthStateSweeper(shutdownCtx)
	api.StartResolverCacheSweeper(shutdownCtx)
	middleware.StartBackgroundCleanups(shutdownCtx)
	defer stopSignals()

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("站点操作管理系统启动，端口: %s (trusted_proxies=%v)", port, trusted)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			log.Fatalf("启动失败: %v", err)
		}
	case <-shutdownCtx.Done():
		log.Println("收到关停信号，等待在途请求完成 (最多 30s)...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("HTTP shutdown 失败: %v", err)
		}
		log.Println("已优雅关停")
	}
}
