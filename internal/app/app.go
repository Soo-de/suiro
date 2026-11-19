// Package app wires each program from its config and runs it until the context
// is canceled. Mains only load config and call one Run function, so the
// all-in-one mode and end-to-end tests start the same code the services run.
package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/Soo-de/suiro/internal/config"
)

// ErrNotImplemented marks a program whose wiring is not built yet.
var ErrNotImplemented = errors.New("not implemented yet")

// RunServer runs the user-facing server: HTTP/JSON API, embedded UI, analysis.
func RunServer(ctx context.Context, cfg config.Server, log *slog.Logger) error {
	log.InfoContext(ctx, "starting", "http_addr", cfg.HTTPAddr)
	return ErrNotImplemented
}

// RunGitCollector runs the service that reads repositories for the server.
func RunGitCollector(ctx context.Context, cfg config.GitCollector, log *slog.Logger) error {
	log.InfoContext(ctx, "starting", "grpc_addr", cfg.GRPCAddr)
	return ErrNotImplemented
}

// RunRenderer runs the service that renders untrusted manifests in isolation.
func RunRenderer(ctx context.Context, cfg config.Renderer, log *slog.Logger) error {
	log.InfoContext(ctx, "starting", "grpc_addr", cfg.GRPCAddr)
	return ErrNotImplemented
}

// RunCloudCollector runs the service that reads the AWS network.
func RunCloudCollector(ctx context.Context, cfg config.CloudCollector, log *slog.Logger) error {
	log.InfoContext(ctx, "starting", "grpc_addr", cfg.GRPCAddr)
	return ErrNotImplemented
}
