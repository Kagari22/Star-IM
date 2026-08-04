package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"IM_Chat_System/internal/app"
)

// .\scripts\Start-Dev.ps1 -WithInfra

func main() {
	a, err := app.New()
	if err != nil {
		log.Fatal(err)
	}
	defer a.Close()

	// 创建 HTTP 服务器
	server := &http.Server{
		Addr:              a.Config.HTTPAddr,
		Handler:           a.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Println("server listening on", a.Config.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	// 注册需要监听的退出信号
	// os.Interrupt -> 用户在终端按 Ctrl + C
	// syscall.SIGTERM -> 操作系统、Docker、进程管理器请求程序正常退出
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
