// Command suiro is the CLI. It analyses a repository locally, or asks the
// server for a pull request's result, and prints JSON or a short table.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Soo-de/suiro/internal/version"
)

var errUsage = errors.New("usage")

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, errUsage) {
			fmt.Fprintln(os.Stderr, "suiro:", err)
		}
		os.Exit(2)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		// The usage error is what the caller acts on; a failed write to stderr adds nothing.
		_ = usage(stderr)
		return errUsage
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version.Version)
		return err
	case "help", "-h", "--help":
		return usage(stdout)
	default:
		_ = usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) error {
	_, err := fmt.Fprint(w, `Usage: suiro <command>

Commands:
  version   print the build version
  help      show this help
`)
	return err
}
