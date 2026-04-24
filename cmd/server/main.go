package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"
)

func main() {
	cfg, err := loadConfig(os.Args[1:])
	if errors.Is(err, pflag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		slog.Default().Error("load standalone config failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app := newApp(cfg)
	report := app.ValidateReport()
	if err := report.Err(); err != nil {
		slog.Default().Error("standalone app validation failed", "err", err)
		os.Exit(1)
	}

	logger := defaultLogger()
	for _, warning := range report.Warnings.Values() {
		logger.Warn("standalone app validation warning", "kind", warning.Kind, "module", warning.Module, "label", warning.Label)
	}

	if err := app.RunContext(ctx); err != nil {
		logger.Error("standalone app exited with error", "err", err)
		os.Exit(1)
	}
}
