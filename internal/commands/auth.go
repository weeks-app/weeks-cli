// Package commands holds the weeks command tree's leaves.
package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// NewAuthCmd builds `weeks auth`.
func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign in to a weeks installation and inspect the session",
		Long: "Authenticate against a weeks installation's OAuth provider.\n\n" +
			"The default is the device authorization grant: weeks prints a short code and a URL,\n" +
			"you approve it in any browser — on any machine — and the CLI picks up the token.\n" +
			"That is the flow that works where an agent usually runs, with no browser of its own\n" +
			"and no way to receive a redirect. Use --browser when there is a desktop to hand off to.",
	}
	cmd.AddCommand(newAuthLoginCmd(), newAuthLogoutCmd(), newAuthStatusCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var (
		browser bool
		scope   string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in and store the credential for this profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			if app.ClientID == "" {
				return &output.Error{
					Code:    output.CodeUsage,
					Message: "no OAuth client id is configured for " + app.BaseURL,
					Hint: "Set " + config.EnvClientID + " to the uid of a Platform::Application on that installation, " +
						"or store it on a profile with `weeks profile set`.",
				}
			}

			// A login waits on a human. Ctrl-C has to end the wait, not leave
			// a polling loop running until the device code expires.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			client := auth.NewClient(app.BaseURL, app.ClientID)

			var (
				creds *auth.Credentials
				err   error
			)
			if browser {
				creds, err = loginViaBrowser(ctx, cmd, app, client, scope)
			} else {
				creds, err = loginViaDevice(ctx, cmd, app, client, scope)
			}
			if err != nil {
				return loginError(err)
			}

			if err := app.Creds().Save(app.Profile, creds); err != nil {
				return fmt.Errorf("could not store the credential: %w", err)
			}

			data := map[string]any{
				"base_url":   creds.BaseURL,
				"profile":    app.Profile,
				"storage":    storageName(app.Creds()),
				"scope":      creds.Scope,
				"expires_at": expiryOrNil(creds),
				"grant":      grantName(browser),
				"client_id":  creds.ClientID,
			}

			opts := []output.ResponseOption{
				output.WithSummary(fmt.Sprintf("Signed in to %s%s.", creds.BaseURL, profileSuffix(app.Profile))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "verify", Cmd: "weeks auth status", Description: "Confirm the stored credential still works"},
					output.Breadcrumb{Action: "diagnose", Cmd: "weeks doctor --json", Description: "Check config, credentials, and connectivity"},
					output.Breadcrumb{Action: "discover", Cmd: "weeks commands --json", Description: "List every command this binary offers"},
				),
			}
			if warning := app.Creds().FallbackWarning(); warning != "" {
				opts = append(opts, output.WithNotice(warning))
			}
			return app.Out.OK(data, opts...)
		},
	}

	cmd.Flags().BoolVar(&browser, "browser", false, "Use the authorization code grant with PKCE and a local redirect")
	cmd.Flags().StringVar(&scope, "scope", "", "OAuth scopes to request (default: the provider's own default)")
	return cmd
}

// loginViaDevice runs the device authorization grant, telling the user what to
// do while it polls.
func loginViaDevice(ctx context.Context, cmd *cobra.Command, app *appctx.App, client *auth.Client, scope string) (*auth.Credentials, error) {
	da, err := client.RequestDeviceCode(ctx, scope)
	if err != nil {
		return nil, err
	}

	// The prompt goes to stderr so that stdout stays a clean envelope even
	// while a human is being talked to. Every write here is discarded on
	// error deliberately: it is a progress note, there is nowhere better to
	// report a failed write to stderr, and losing one must not fail a login
	// that is otherwise going fine.
	w := cmd.ErrOrStderr()
	say(w, "\n  Open %s\n", verificationTarget(da))
	say(w, "  Enter the code: %s\n\n", da.UserCode)
	if da.VerificationURIComplete != "" && app.Interactive {
		// Opening the browser is a convenience, never the instruction: the URL
		// and code above are what the user actually needs, and this machine
		// may have no desktop at all.
		_ = auth.OpenBrowser(da.VerificationURIComplete)
	}
	say(w, "  Waiting for approval (the code expires in %s)…\n", time.Duration(da.ExpiresIn)*time.Second)

	return client.PollDeviceToken(ctx, da, func(next time.Duration) {
		if app.Verbose {
			say(w, "  The server asked for a slower poll; now every %s.\n", next)
		}
	})
}

// loginViaBrowser runs the authorization code grant with PKCE.
func loginViaBrowser(ctx context.Context, cmd *cobra.Command, app *appctx.App, client *auth.Client, scope string) (*auth.Credentials, error) {
	w := cmd.ErrOrStderr()
	return auth.BrowserLogin(ctx, client, scope, func(url string) {
		say(w, "\n  Opening %s\n\n", url)
		if app.Interactive {
			_ = auth.OpenBrowser(url)
		}
	})
}

// verificationTarget prefers the URL that already carries the code, since it
// spares the user a transcription, and falls back to the plain one.
func verificationTarget(da *auth.DeviceAuthorization) string {
	if da.VerificationURIComplete != "" {
		return da.VerificationURIComplete
	}
	return da.VerificationURI
}

// loginError turns the flow's outcomes into codes a caller can branch on.
// Denial and expiry are not API errors — they are answers.
func loginError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return &output.Error{Code: output.CodeUsage, Message: "login was canceled"}
	case errors.Is(err, auth.ErrDeviceDenied):
		return &output.Error{
			Code:    output.CodeForbidden,
			Message: "the authorization request was denied",
			Hint:    "Run `weeks auth login` again and approve the code.",
		}
	case errors.Is(err, auth.ErrDeviceExpired):
		return &output.Error{
			Code:    output.CodeAuth,
			Message: "the device code expired before it was approved",
			Hint:    "Run `weeks auth login` again; the new code is good for a few minutes.",
		}
	default:
		return err
	}
}

func newAuthLogoutCmd() *cobra.Command {
	var revoke bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored credential for this profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)

			creds, err := app.Creds().Load(app.Profile, app.BaseURL)
			if err != nil {
				return &output.Error{
					Code:    output.CodeAuth,
					Message: fmt.Sprintf("no stored credential for %s%s", app.BaseURL, profileSuffix(app.Profile)),
					Hint:    "Nothing to forget. `weeks auth login` signs in.",
				}
			}

			var notice string
			if revoke {
				// A provider that does not mount /oauth/revoke should not stop
				// the local credential being forgotten — the user asked for
				// the token to go away, and half of that always succeeds.
				if err := auth.NewClient(app.BaseURL, app.ClientID).Revoke(cmd.Context(), creds.AccessToken); err != nil {
					notice = fmt.Sprintf("the credential was forgotten locally, but the server did not revoke it: %v", err)
				}
			}

			if err := app.Creds().Delete(app.Profile, app.BaseURL); err != nil {
				return fmt.Errorf("could not remove the credential: %w", err)
			}

			opts := []output.ResponseOption{
				output.WithSummary(fmt.Sprintf("Signed out of %s%s.", app.BaseURL, profileSuffix(app.Profile))),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "login", Cmd: "weeks auth login", Description: "Sign in again",
				}),
			}
			if notice != "" {
				opts = append(opts, output.WithNotice(notice))
			}
			return app.Out.OK(map[string]any{
				"base_url": app.BaseURL,
				"profile":  app.Profile,
				"revoked":  revoke && notice == "",
			}, opts...)
		},
	}

	cmd.Flags().BoolVar(&revoke, "revoke", false, "Also ask the server to revoke the token")
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report whether this profile has a usable credential",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)

			creds, err := app.Creds().Load(app.Profile, app.BaseURL)
			if err != nil {
				return &output.Error{
					Code:    output.CodeAuth,
					Message: fmt.Sprintf("not signed in to %s%s", app.BaseURL, profileSuffix(app.Profile)),
					Hint:    "Run `weeks auth login`.",
				}
			}

			if creds.Expired() {
				return &output.Error{
					Code:    output.CodeAuth,
					Message: fmt.Sprintf("the credential for %s expired at %s", creds.BaseURL, creds.ExpiresAt.Format(time.RFC3339)),
					Hint:    "Run `weeks auth login` to sign in again.",
				}
			}

			return app.Out.OK(map[string]any{
				"authenticated": true,
				"base_url":      creds.BaseURL,
				"profile":       app.Profile,
				"storage":       storageName(app.Creds()),
				"scope":         creds.Scope,
				"expires_at":    expiryOrNil(creds),
				"created_at":    creds.CreatedAt.Format(time.RFC3339),
				"client_id":     creds.ClientID,
			},
				output.WithSummary(fmt.Sprintf("Signed in to %s%s.", creds.BaseURL, profileSuffix(app.Profile))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "diagnose", Cmd: "weeks doctor --json", Description: "Check config, credentials, and connectivity"},
					output.Breadcrumb{Action: "discover", Cmd: "weeks commands --json", Description: "List every command this binary offers"},
				),
			)
		},
	}
}

// say writes a progress note for a watching human. The error is dropped on
// purpose — see loginViaDevice.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// storageName names where the credential actually lives, which is the first
// thing to know when a login "worked" but the next command cannot find it.
func storageName(s *auth.Store) string {
	if s.UsingKeyring() {
		return "keyring"
	}
	return "file"
}

// expiryOrNil renders an expiry as RFC 3339, or null when the provider issued
// a token that does not expire. An empty string would read as "expired now".
func expiryOrNil(c *auth.Credentials) any {
	if c.ExpiresAt.IsZero() {
		return nil
	}
	return c.ExpiresAt.Format(time.RFC3339)
}

func profileSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " as profile " + name
}

func grantName(browser bool) string {
	if browser {
		return "authorization_code"
	}
	return "device_code"
}
