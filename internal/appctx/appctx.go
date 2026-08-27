// Package appctx carries the resolved per-invocation context — which
// installation, as whom, and how to answer — from the root command down to
// every leaf.
//
// It is its own package so that command packages can read the context without
// importing the package that builds the command tree, which imports them.
package appctx

import (
	"context"

	bcprofile "github.com/basecamp/cli/profile"
	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// App is what a command needs to know about the invocation it is part of.
// Every command reads it instead of re-deriving the same things from flags
// and environment.
type App struct {
	Out *output.Writer

	// Profile is the resolved profile name, empty when none is configured.
	Profile string

	// BaseURL is the weeks installation this invocation talks to.
	BaseURL string

	// ClientID is the OAuth client the login flows authenticate as.
	ClientID string

	// Agent is true when the caller asked for the agent-shaped surface
	// (--agent, or --json): help becomes JSON and prose becomes data.
	Agent bool

	// Confirm is true when the caller passed --confirm, clearing the
	// confirmation gate for this invocation.
	Confirm bool

	// Verbose is true when the caller asked for extra detail.
	Verbose bool

	// Interactive is true when stdout is a terminal and the caller did not ask
	// for a machine format. A device-flow login prints a code and waits when
	// this is true; otherwise it says everything up front and still waits.
	Interactive bool

	Creds    *auth.Store
	Profiles *bcprofile.Store
}

type key struct{}

// With returns ctx carrying app.
func With(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, key{}, app)
}

// From returns the App carried by cmd's context.
//
// Root command setup installs one before any RunE fires, so a missing App is a
// programming error rather than a user-facing condition. Returning a usable
// zero value keeps a mis-wired command printing a real error instead of
// panicking on a nil writer.
func From(cmd *cobra.Command) *App {
	if app, ok := cmd.Context().Value(key{}).(*App); ok && app != nil {
		return app
	}
	return &App{Out: output.New(output.DefaultOptions())}
}
