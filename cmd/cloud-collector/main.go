// Command cloud-collector reads the AWS network read-only. It never sees PR content.
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

	cfg, err := config.LoadCloudCollector(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cloud-collector: config:", err)
		os.Exit(2)
	}
	log := telemetry.NewLogger(os.Stdout, cfg.LogLevel, "cloud-collector")
	if err := app.RunCloudCollector(ctx, cfg, log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}
