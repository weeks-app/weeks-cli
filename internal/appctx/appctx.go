// Package appctx carries the resolved per-invocation context — which
// installation, as whom, and how to answer — from the root command down to
// every leaf.
//
// It is its own package so that command packages can read the context without
// importing the package that builds the command tree, which imports them.
package appctx

import (
	"context"
	"sync"

	bcprofile "github.com/basecamp/cli/profile"
	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
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

	// ConfigDir is where profiles and file-backed credentials are read/written.
	ConfigDir string

	// ConfigScope names how ConfigDir was selected: local, global, or env.
	ConfigScope string

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

	Profiles *bcprofile.Store

	credsOnce sync.Once
	creds     *auth.Store
}

// Creds opens the credential store, once, on first use.
//
// Opening it probes the system keyring, and that probe is unbounded in the
// toolkit's current release: on a locked keychain it does not return. Building
// it eagerly meant `weeks version` and `weeks commands --json` — which never
// look at a credential — hung on a machine whose keychain was locked, which is
// most of the machines an agent runs on. Nothing pays for the keyring until
// something actually needs a token.
func (a *App) Creds() *auth.Store {
	a.credsOnce.Do(func() {
		if a.ConfigScope == config.ScopeLocal || a.ConfigScope == config.ScopeEnv {
			a.creds = auth.NewFileStore(a.ConfigDir)
		} else {
			a.creds = auth.NewStore(a.ConfigDir)
		}
	})
	return a.creds
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
	if app, ok := Lookup(cmd.Context()); ok {
		return app
	}
	return &App{Out: output.New(output.DefaultOptions())}
}

// Lookup returns the App carried by ctx, if root setup has installed one.
func Lookup(ctx context.Context) (*App, bool) {
	app, ok := ctx.Value(key{}).(*App)
	return app, ok && app != nil
}
