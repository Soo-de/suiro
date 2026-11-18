// Package version reports the build's version. The Makefile sets it at link
// time (-X github.com/Soo-de/suiro/internal/version.Version=...). Analyses
// record it, so a changed result can be traced back to a changed tool.
package version

// Version is "dev" for builds that did not go through the Makefile.
var Version = "dev"
