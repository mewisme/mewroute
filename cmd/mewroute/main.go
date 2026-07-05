package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mewisme/mewroute/internal/app"
	"github.com/mewisme/mewroute/internal/config"
	"github.com/mewisme/mewroute/internal/logx"
)

func main() {
	env, err := config.LoadEnv()
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	logger := logx.New(env.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, env, logger); err != nil {
		logger.Error("fatal error", "error", err)
		os.Exit(1)
	}
}
