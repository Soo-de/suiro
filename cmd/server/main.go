// Command server is the user-facing server: HTTP/JSON API, embedded UI and
// analysis. It holds only the database connection.
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

	cfg, err := config.LoadServer(os.LookupEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "server: config:", err)
		os.Exit(2)
	}
	log := telemetry.NewLogger(os.Stdout, cfg.LogLevel, "server")
	if err := app.RunServer(ctx, cfg, log); err != nil {
		log.Error("exiting", "err", err)
		os.Exit(1)
	}
}
