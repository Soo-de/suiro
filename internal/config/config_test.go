package config

import (
	"log/slog"
	"testing"
)

func envOf(m map[string]string) Lookup {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestLoadServerDefaults(t *testing.T) {
	got, err := LoadServer(envOf(nil))
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if got.HTTPAddr != ":8080" || got.LogLevel != slog.LevelInfo {
		t.Fatalf("got %+v, want :8080 and info", got)
	}
}

func TestBlankValueFallsBackToDefault(t *testing.T) {
	got, err := LoadGitCollector(envOf(map[string]string{"SUIRO_GRPC_ADDR": "  "}))
	if err != nil {
		t.Fatalf("LoadGitCollector: %v", err)
	}
	if got.GRPCAddr != ":9090" {
		t.Fatalf("GRPCAddr = %q, want :9090", got.GRPCAddr)
	}
}

func TestSettingsAreRead(t *testing.T) {
	got, err := LoadServer(envOf(map[string]string{
		"SUIRO_HTTP_ADDR": "127.0.0.1:9000",
		"SUIRO_LOG_LEVEL": "debug",
	}))
	if err != nil {
		t.Fatalf("LoadServer: %v", err)
	}
	if got.HTTPAddr != "127.0.0.1:9000" || got.LogLevel != slog.LevelDebug {
		t.Fatalf("got %+v", got)
	}
}

func TestInvalidLogLevelIsAnError(t *testing.T) {
	if _, err := LoadCommon(envOf(map[string]string{"SUIRO_LOG_LEVEL": "loud"})); err == nil {
		t.Fatal("want an error for an unknown log level")
	}
}
