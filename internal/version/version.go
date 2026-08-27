// Package version carries the build identity of the binary.
package version

// Version is the release version, overwritten at build time with
// -ldflags "-X github.com/weeks-app/weeks-cli/internal/version.Version=v1.2.3".
var Version = "dev"

// Commit is the git SHA the binary was built from, set the same way.
var Commit = "none"

// Date is the build timestamp, set the same way.
var Date = "unknown"
