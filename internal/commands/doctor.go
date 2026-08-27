package commands

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/harness"
	"github.com/weeks-app/weeks-cli/internal/output"
	"github.com/weeks-app/weeks-cli/internal/version"
)

// Check statuses. A check that could not run is "skip", not "fail": telling
// someone their credential is broken when the real answer is "you are not
// signed in yet" sends them to fix the wrong thing.
const (
	StatusPass = "pass"
	StatusFail = "fail"
	StatusWarn = "warn"
	StatusSkip = "skip"
)

// Check is one diagnostic result.
type Check struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// DoctorResult is the whole run.
type DoctorResult struct {
	Checks  []Check `json:"checks"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Warned  int     `json:"warned"`
	Skipped int     `json:"skipped"`
	Healthy bool    `json:"healthy"`
}

// NewDoctorCmd builds `weeks doctor`.
func NewDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration, credentials, and connectivity",
		Long: "Run the checks that explain why weeks is not working.\n\n" +
			"Each check answers one question and, when it fails, names the command that fixes it.\n" +
			"`weeks doctor --json` is the first thing an agent should run when a command surprises it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			result := runChecks(cmd.Context(), app)

			opts := []output.ResponseOption{
				output.WithSummary(doctorSummary(result)),
				output.WithBreadcrumbs(doctorBreadcrumbs(result)...),
			}

			if err := app.Out.OK(result, opts...); err != nil {
				return err
			}

			// A failing doctor must not exit 0: a CI step or an agent that
			// reads only the exit code has to see the problem. The envelope
			// above is the whole answer, so this adds the status and nothing
			// else — two envelopes would leave the caller guessing which one
			// is the result.
			if !result.Healthy {
				return &output.SilentExit{Code: output.ExitAPI}
			}
			return nil
		},
	}
}

func runChecks(ctx context.Context, app *appctx.App) *DoctorResult {
	result := &DoctorResult{}

	add := func(c Check) {
		result.Checks = append(result.Checks, c)
		switch c.Status {
		case StatusPass:
			result.Passed++
		case StatusFail:
			result.Failed++
		case StatusWarn:
			result.Warned++
		case StatusSkip:
			result.Skipped++
		}
	}

	add(checkVersion())
	add(checkConfigDir())
	add(checkProfile(app))

	credCheck, creds := checkCredentials(app)
	add(credCheck)

	add(checkConnectivity(ctx, app))
	add(checkAPIAccess(ctx, app, creds != nil))
	add(harnessCheck())

	result.Healthy = result.Failed == 0
	return result
}

func checkVersion() Check {
	return Check{
		ID:      "version",
		Name:    "CLI version",
		Status:  StatusPass,
		Message: fmt.Sprintf("weeks %s (%s)", version.Version, version.Commit),
	}
}

func checkConfigDir() Check {
	dir := config.Dir()

	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		// Nothing has been configured yet. That is the state of a fresh
		// install, not a fault.
		return Check{
			ID: "config", Name: "Config directory", Status: StatusSkip,
			Message: dir + " does not exist yet",
			Hint:    "It is created on the first `weeks auth login` or `weeks profile set`.",
		}
	}
	if err != nil {
		return Check{
			ID: "config", Name: "Config directory", Status: StatusFail,
			Message: fmt.Sprintf("cannot read %s: %v", dir, err),
		}
	}
	if !info.IsDir() {
		return Check{
			ID: "config", Name: "Config directory", Status: StatusFail,
			Message: dir + " exists but is not a directory",
			Hint:    "Move it aside, or point " + config.EnvConfigDir + " somewhere else.",
		}
	}

	// 0700 is what the profile store creates. Anything wider means another
	// account on this machine can read a plaintext credential fallback.
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return Check{
			ID: "config", Name: "Config directory", Status: StatusWarn,
			Message: fmt.Sprintf("%s is mode %04o, which is readable by other users", dir, perm),
			Hint:    fmt.Sprintf("chmod 700 %s", dir),
		}
	}

	return Check{ID: "config", Name: "Config directory", Status: StatusPass, Message: dir}
}

func checkProfile(app *appctx.App) Check {
	profiles, defaultName, err := app.Profiles.List()
	if err != nil {
		return Check{
			ID: "profile", Name: "Profiles", Status: StatusFail,
			Message: fmt.Sprintf("cannot read %s: %v", config.ProfilesPath(), err),
			Hint:    "The file is not valid JSON. Fix or remove it.",
		}
	}

	switch {
	case len(profiles) == 0:
		return Check{
			ID: "profile", Name: "Profiles", Status: StatusSkip,
			Message: "no profiles configured; commands target " + app.BaseURL,
			Hint:    "`weeks profile set <name> --base-url <url>` if you work with more than one installation.",
		}
	case app.Profile == "":
		return Check{
			ID: "profile", Name: "Profiles", Status: StatusWarn,
			Message: fmt.Sprintf("%d profiles configured but none is active", len(profiles)),
			Hint:    "Pass --profile, set " + config.EnvProfile + ", or run `weeks profile default <name>`.",
		}
	default:
		msg := fmt.Sprintf("%s (of %d)", app.Profile, len(profiles))
		if app.Profile == defaultName {
			msg += ", the default"
		}
		return Check{ID: "profile", Name: "Profiles", Status: StatusPass, Message: msg}
	}
}

// checkCredentials also returns the credentials it found, so the API-access
// check does not have to load them a second time.
func checkCredentials(app *appctx.App) (Check, *credentialsSummary) {
	storage := storageName(app.Creds())

	creds, err := app.Creds().Load(app.Profile, app.BaseURL)
	if err != nil {
		return Check{
			ID: "credentials", Name: "Credentials", Status: StatusSkip,
			Message: fmt.Sprintf("not signed in to %s%s", app.BaseURL, profileSuffix(app.Profile)),
			Hint:    "Run `weeks auth login`.",
		}, nil
	}

	if creds.Expired() {
		return Check{
			ID: "credentials", Name: "Credentials", Status: StatusFail,
			Message: fmt.Sprintf("the %s credential expired at %s", storage, creds.ExpiresAt.Format(time.RFC3339)),
			Hint:    "Run `weeks auth login` to sign in again.",
		}, nil
	}

	summary := &credentialsSummary{token: creds.AccessToken}

	if !app.Creds().UsingKeyring() {
		// File storage that was asked for is a decision, not a fault — a
		// headless box or a container has no keyring to reach, and warning
		// about it every run trains people to ignore the warnings that matter.
		if os.Getenv(config.EnvNoKeyring) != "" {
			return Check{
				ID: "credentials", Name: "Credentials", Status: StatusPass,
				Message: fmt.Sprintf("stored in a 0600 file at %s, because %s is set", config.Dir(), config.EnvNoKeyring),
			}, summary
		}
		warning := app.Creds().FallbackWarning()
		if warning == "" {
			warning = "the system keyring is not in use; credentials are stored in a 0600 file at " + config.Dir()
		}
		return Check{
			ID: "credentials", Name: "Credentials", Status: StatusWarn,
			Message: warning,
			Hint:    "Unlock or install a system keyring, or set " + config.EnvNoKeyring + " to choose file storage deliberately.",
		}, summary
	}

	msg := "stored in the system keyring"
	if creds.ExpiringWithin(24 * time.Hour) {
		return Check{
			ID: "credentials", Name: "Credentials", Status: StatusWarn,
			Message: fmt.Sprintf("%s, expiring %s", msg, creds.ExpiresAt.Format(time.RFC3339)),
			Hint:    "Run `weeks auth login` before it lapses.",
		}, summary
	}
	return Check{ID: "credentials", Name: "Credentials", Status: StatusPass, Message: msg}, summary
}

type credentialsSummary struct{ token string }

func checkConnectivity(ctx context.Context, app *appctx.App) Check {
	client := &http.Client{Timeout: 10 * time.Second}

	// The OAuth authorization endpoint is the cheapest proof that this is a
	// weeks installation and not merely a host that answers on 443: any
	// response at all means Doorkeeper is mounted where the CLI expects it.
	url := app.BaseURL + "/oauth/authorize"

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return Check{ID: "connectivity", Name: "Connectivity", Status: StatusFail, Message: err.Error()}
	}

	resp, err := client.Do(req)
	if err != nil {
		return Check{
			ID: "connectivity", Name: "Connectivity", Status: StatusFail,
			Message: fmt.Sprintf("cannot reach %s: %v", app.BaseURL, err),
			Hint:    "Check the base URL, and whether the server is running. In a worktree collection it is on WEEKS_APP_PORT.",
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		return Check{
			ID: "connectivity", Name: "Connectivity", Status: StatusFail,
			Message: fmt.Sprintf("%s answered %s", app.BaseURL, resp.Status),
		}
	}
	return Check{
		ID: "connectivity", Name: "Connectivity", Status: StatusPass,
		Message: fmt.Sprintf("%s answered %s", app.BaseURL, resp.Status),
	}
}

func checkAPIAccess(ctx context.Context, app *appctx.App, haveCreds bool) Check {
	if !haveCreds {
		return Check{
			ID: "api", Name: "API access", Status: StatusSkip,
			Message: "no credential to test with",
			Hint:    "Run `weeks auth login`.",
		}
	}

	creds, err := app.Creds().Load(app.Profile, app.BaseURL)
	if err != nil {
		return Check{ID: "api", Name: "API access", Status: StatusSkip, Message: "no credential to test with"}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, app.BaseURL+"/api/v1/teams", nil)
	if err != nil {
		return Check{ID: "api", Name: "API access", Status: StatusFail, Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return Check{
			ID: "api", Name: "API access", Status: StatusFail,
			Message: fmt.Sprintf("request failed: %v", err),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return Check{
			ID: "api", Name: "API access", Status: StatusFail,
			Message: "the stored token was rejected",
			Hint:    "Run `weeks auth login` to sign in again.",
		}
	case resp.StatusCode == http.StatusForbidden:
		return Check{
			ID: "api", Name: "API access", Status: StatusFail,
			Message: "the token is valid but not permitted to read teams",
			Hint:    "Check the scopes and the role of the platform application this token belongs to.",
		}
	case resp.StatusCode >= 400:
		return Check{
			ID: "api", Name: "API access", Status: StatusFail,
			Message: fmt.Sprintf("the API answered %s", resp.Status),
		}
	default:
		return Check{
			ID: "api", Name: "API access", Status: StatusPass,
			Message: fmt.Sprintf("authenticated request answered %s", resp.Status),
		}
	}
}

func harnessCheck() Check {
	c := harness.CheckClaudePlugin()
	return Check{ID: "claude-plugin", Name: c.Name, Status: c.Status, Message: c.Message, Hint: c.Hint}
}

func doctorSummary(r *DoctorResult) string {
	if r.Healthy && r.Warned == 0 {
		return fmt.Sprintf("All %d checks passed.", r.Passed)
	}
	parts := []string{}
	if r.Failed > 0 {
		parts = append(parts, plural(r.Failed, "check", "checks")+" failed")
	}
	parts = append(parts, fmt.Sprintf("%d passed", r.Passed))
	if r.Warned > 0 {
		parts = append(parts, plural(r.Warned, "warning", "warnings"))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", r.Skipped))
	}
	return strings.Join(parts, ", ") + "."
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// doctorBreadcrumbs suggests the command that addresses what actually went
// wrong, rather than a fixed list.
func doctorBreadcrumbs(r *DoctorResult) []output.Breadcrumb {
	var crumbs []output.Breadcrumb
	for _, c := range r.Checks {
		if c.Status != StatusFail && c.Status != StatusSkip {
			continue
		}
		switch c.ID {
		case "credentials", "api":
			crumbs = append(crumbs, output.Breadcrumb{
				Action: "login", Cmd: "weeks auth login", Description: "Sign in to this installation",
			})
		case "claude-plugin":
			crumbs = append(crumbs, output.Breadcrumb{
				Action: "install-skill", Cmd: "weeks setup claude", Description: "Install the weeks agent skill for Claude Code",
			})
		}
	}
	if len(crumbs) == 0 {
		crumbs = append(crumbs, output.Breadcrumb{
			Action: "discover", Cmd: "weeks commands --json", Description: "List every command this binary offers",
		})
	}
	return dedupeBreadcrumbs(crumbs)
}

func dedupeBreadcrumbs(in []output.Breadcrumb) []output.Breadcrumb {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, c := range in {
		if seen[c.Cmd] {
			continue
		}
		seen[c.Cmd] = true
		out = append(out, c)
	}
	return out
}
