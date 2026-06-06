package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      90 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Printf("server listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

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
