// Command renderer renders Helm charts, Kustomize overlays and plain YAML from untrusted PR content, without credentials or network.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Soo-de/suiro/internal/app"
	"github.com/Soo-de/suiro/internal/config"
	"github.com/Soo-de/suiro/internal/telemetry"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadRenderer(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "renderer: config:", err)
		os.Exit(2)
	}
	log := telemetry.NewLogger(os.Stdout, cfg.LogLevel, "renderer")
	if err := app.RunRenderer(ctx, cfg, log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}
