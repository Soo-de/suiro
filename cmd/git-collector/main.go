// Command git-collector reads registered repositories for the server. It is
// the only service with GitHub credentials.
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

	cfg, err := config.LoadGitCollector(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "git-collector: config:", err)
		os.Exit(2)
	}
	log := telemetry.NewLogger(os.Stdout, cfg.LogLevel, "git-collector")
	if err := app.RunGitCollector(ctx, cfg, log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}
