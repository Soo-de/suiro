package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionPrintsBuildVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"version"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run version: %v", err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("version printed nothing")
	}
}

func TestUnknownCommandIsAnError(t *testing.T) {
	if err := run([]string{"nope"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("want an error for an unknown command")
	}
}

func TestNoArgumentsShowsUsage(t *testing.T) {
	var errOut bytes.Buffer
	if err := run(nil, &bytes.Buffer{}, &errOut); err == nil {
		t.Fatal("want a usage error without arguments")
	}
	if !strings.Contains(errOut.String(), "Usage") {
		t.Fatalf("usage not shown: %q", errOut.String())
	}
}
