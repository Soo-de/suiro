// Package telemetry sets up what the programs report about themselves: JSON
// logs on stdout for the cluster's log collector, and later request IDs that
// follow a request across services and Prometheus metrics on /metrics.
package telemetry

import (
	"io"
	"log/slog"

	"github.com/Soo-de/suiro/internal/version"
)

// NewLogger returns a JSON logger that tags every line with the service name
// and build version.
func NewLogger(w io.Writer, level slog.Level, service string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(h).With("service", service, "version", version.Version)
}
