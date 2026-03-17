package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/lev/internet-monitor/internal/config"
	"github.com/lev/internet-monitor/internal/monitor"
	"github.com/lev/internet-monitor/internal/store"
	"github.com/lev/internet-monitor/internal/web"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fileStore := store.New(cfg.DataDir, cfg.RetentionDays)
	if err := fileStore.EnsureLayout(); err != nil {
		log.Fatalf("prepare storage: %v", err)
	}

	service := monitor.NewService(cfg, fileStore)
	go service.Run(ctx)

	handler := web.NewServer(cfg, fileStore)
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("http shutdown: %v", err)
		}
	}()

	log.Printf("internet monitor listening on %s", cfg.ListenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server: %v", err)
	}
}
