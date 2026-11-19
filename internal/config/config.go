// Package config is the only place that reads the process environment. Each
// program loads its typed settings here and passes them down; no other package
// calls os.Getenv, so every setting is listed in this package and, without a
// value, in .env.example.
package config

import (
	"fmt"
	"log/slog"
	"strings"
)

// Lookup returns a setting's value and whether it was set; os.LookupEnv fits.
type Lookup func(key string) (string, bool)

// Common holds the settings every program shares.
type Common struct {
	LogLevel slog.Level
}

// Server configures the user-facing server.
type Server struct {
	Common
	HTTPAddr string
}

// GitCollector configures the service that reads repositories.
type GitCollector struct {
	Common
	GRPCAddr string
}

// Renderer configures the service that renders Helm, Kustomize and plain YAML.
type Renderer struct {
	Common
	GRPCAddr string
}

// CloudCollector configures the service that reads the AWS network.
type CloudCollector struct {
	Common
	GRPCAddr string
}

const (
	defaultHTTPAddr = ":8080"
	defaultGRPCAddr = ":9090"
)

// LoadCommon reads the settings shared by every program.
func LoadCommon(env Lookup) (Common, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(get(env, "SUIRO_LOG_LEVEL", "info"))); err != nil {
		return Common{}, fmt.Errorf("SUIRO_LOG_LEVEL: %w", err)
	}
	return Common{LogLevel: level}, nil
}

// LoadServer reads the server's settings.
func LoadServer(env Lookup) (Server, error) {
	c, err := LoadCommon(env)
	if err != nil {
		return Server{}, err
	}
	return Server{Common: c, HTTPAddr: get(env, "SUIRO_HTTP_ADDR", defaultHTTPAddr)}, nil
}

// LoadGitCollector reads git-collector's settings.
func LoadGitCollector(env Lookup) (GitCollector, error) {
	c, err := LoadCommon(env)
	if err != nil {
		return GitCollector{}, err
	}
	return GitCollector{Common: c, GRPCAddr: get(env, "SUIRO_GRPC_ADDR", defaultGRPCAddr)}, nil
}

// LoadRenderer reads the renderer's settings.
func LoadRenderer(env Lookup) (Renderer, error) {
	c, err := LoadCommon(env)
	if err != nil {
		return Renderer{}, err
	}
	return Renderer{Common: c, GRPCAddr: get(env, "SUIRO_GRPC_ADDR", defaultGRPCAddr)}, nil
}

// LoadCloudCollector reads cloud-collector's settings.
func LoadCloudCollector(env Lookup) (CloudCollector, error) {
	c, err := LoadCommon(env)
	if err != nil {
		return CloudCollector{}, err
	}
	return CloudCollector{Common: c, GRPCAddr: get(env, "SUIRO_GRPC_ADDR", defaultGRPCAddr)}, nil
}

// get treats a blank value like an unset one, so an empty line in .env falls
// back to the default instead of becoming an empty address.
func get(env Lookup, key, fallback string) string {
	if v, ok := env(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
