package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mini-atoms/internal/app"
	"mini-atoms/internal/config"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	srv, err := app.New(cfg)
	if err != nil {
		logger.Fatalf("init app: %v", err)
	}

	logger.Printf("mini-atoms listening on %s", cfg.HTTPAddr)
	if err := srv.Run(ctx); err != nil {
		logger.Fatalf("server exited with error: %v", err)
	}

	logger.Printf("server stopped")
}
