package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"walkietalk-go/internal/ai"
	"walkietalk-go/internal/api"
	"walkietalk-go/internal/config"
	"walkietalk-go/internal/realtime"
	"walkietalk-go/internal/store"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "walkietalk-go ", log.LstdFlags|log.Lmicroseconds)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rateStore := store.NewRateStore(ctx, cfg, logger)
	defer rateStore.Close()

	aiClient := ai.NewClient(cfg, logger)
	hub := realtime.NewHub(cfg, rateStore, aiClient, logger)
	go hub.Run(ctx)

	server := api.NewServer(cfg, hub, aiClient, rateStore, logger)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 8 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		logger.Printf("server listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	go startKeepAliveLoop(ctx, cfg, logger)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	logger.Println("shutdown requested")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Printf("http shutdown error: %v", err)
	}
	hub.Close()
	logger.Println("server stopped")
}

func startKeepAliveLoop(ctx context.Context, cfg config.Config, logger *log.Logger) {
	if !cfg.KeepAliveEnabled || cfg.KeepAliveURL == "" {
		logger.Printf("keepalive self-ping disabled url_configured=%t enabled=%t", cfg.KeepAliveURL != "", cfg.KeepAliveEnabled)
		return
	}
	target := strings.TrimRight(cfg.KeepAliveURL, "/") + cfg.KeepAlivePath
	client := &http.Client{Timeout: cfg.KeepAliveTimeout}

	ping := func(reason string) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			logger.Printf("keepalive request build failed: %v", err)
			return
		}
		req.Header.Set("User-Agent", "walkietalk-go-keepalive/1.0")
		req.Header.Set("X-Keepalive-Source", reason)
		if cfg.KeepAliveToken != "" {
			req.Header.Set("X-Keep-Alive-Token", cfg.KeepAliveToken)
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() == nil {
				logger.Printf("keepalive ping failed url=%s err=%v", target, err)
			}
			return
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			logger.Printf("keepalive ping non-2xx url=%s status=%d", target, resp.StatusCode)
		}
	}

	logger.Printf("keepalive self-ping enabled url=%s interval=%s", target, cfg.KeepAliveInterval)
	initial := time.NewTimer(25 * time.Second)
	defer initial.Stop()
	select {
	case <-ctx.Done():
		return
	case <-initial.C:
		ping("startup")
	}

	ticker := time.NewTicker(cfg.KeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ping("interval")
		}
	}
}
