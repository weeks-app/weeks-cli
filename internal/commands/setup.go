package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	bcprofile "github.com/basecamp/cli/profile"
	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/harness"
	"github.com/weeks-app/weeks-cli/internal/output"
	"github.com/weeks-app/weeks-cli/skills"
)

// NewSetupCmd builds `weeks setup`.
func NewSetupCmd() *cobra.Command {
	var opts setupOptions

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Set up weeks for this machine and its coding agents",
		Long: "Create a default profile, optionally sign in, and install the embedded agent skill.\n\n" +
			"A CLI without a profile cannot remember where to talk to; a CLI without a skill is one an agent rediscovers every session.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.profile, "profile", "default", "Profile name to create or select")
	cmd.Flags().StringVar(&opts.baseURL, "base-url", "", "weeks installation this profile targets")
	cmd.Flags().StringVar(&opts.clientID, "client-id", "", "OAuth client id for this installation")
	cmd.Flags().BoolVar(&opts.login, "login", false, "Sign in after saving the profile")
	cmd.Flags().BoolVar(&opts.skipProfile, "skip-profile", false, "Do not create or update a profile")
	cmd.Flags().BoolVar(&opts.skipSkill, "skip-skill", false, "Do not install the agent skill")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite existing setup-owned files")

	cmd.AddCommand(newSetupClaudeCmd())
	return cmd
}

type setupOptions struct {
	profile     string
	baseURL     string
	clientID    string
	login       bool
	skipProfile bool
	skipSkill   bool
	force       bool
}

func runSetup(cmd *cobra.Command, opts setupOptions) error {
	app := appctx.From(cmd)
	baseURL := config.NormalizeBaseURL(firstNonEmpty(opts.baseURL, app.BaseURL, config.DefaultBaseURL))
	clientID := firstNonEmpty(opts.clientID, app.ClientID)

	result := map[string]any{
		"profile":       nil,
		"base_url":      baseURL,
		"skill_path":    nil,
		"skill_updated": false,
		"authenticated": false,
	}
	crumbs := []output.Breadcrumb{
		{Action: "doctor", Cmd: "weeks doctor --json", Description: "Check config, credentials, and agent setup"},
		{Action: "discover", Cmd: "weeks commands --json", Description: "List every command this binary offers"},
	}

	profile := opts.profile
	if !opts.skipProfile {
		if err := bcprofile.ValidateName(profile); err != nil {
			return output.ErrUsage(err.Error())
		}
		if err := saveSetupProfile(app, profile, baseURL, clientID); err != nil {
			return err
		}
		result["profile"] = profile
		crumbs = append(crumbs, output.Breadcrumb{
			Action: "teams", Cmd: "weeks teams list --profile " + profile, Description: "List teams this profile can access",
		})
	}

	if !opts.skipSkill {
		install, err := installClaudeSkill(opts.force)
		if err != nil {
			return err
		}
		result["skill_path"] = install.path
		result["skill_updated"] = install.updated
	}

	if opts.login {
		loginApp := &appctx.App{
			Out:         app.Out,
			Profile:     profile,
			BaseURL:     baseURL,
			ClientID:    clientID,
			Agent:       app.Agent,
			Confirm:     app.Confirm,
			Verbose:     app.Verbose,
			Interactive: app.Interactive,
			Profiles:    app.Profiles,
		}
		if loginApp.ClientID == "" {
			return &output.Error{
				Code:    output.CodeUsage,
				Message: "no OAuth client id is configured for " + baseURL,
				Hint:    "Pass --client-id, set " + config.EnvClientID + ", or omit --login and run `weeks auth login` after configuring one.",
			}
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
		defer stop()
		creds, err := loginViaDevice(ctx, cmd, loginApp, auth.NewClient(baseURL, clientID), "")
		if err != nil {
			return loginError(err)
		}
		if err := loginApp.Creds().Save(loginApp.Profile, creds); err != nil {
			return fmt.Errorf("could not store the credential: %w", err)
		}
		result["authenticated"] = true
	} else {
		loginCmd := "weeks auth login"
		if profile != "" {
			loginCmd += " --profile " + profile
		}
		crumbs = append(crumbs, output.Breadcrumb{Action: "login", Cmd: loginCmd, Description: "Sign in when you are ready"})
	}

	return app.Out.OK(result,
		output.WithSummary("weeks setup is ready."),
		output.WithBreadcrumbs(crumbs...),
	)
}

func saveSetupProfile(app *appctx.App, name, baseURL, clientID string) error {
	p := &bcprofile.Profile{Name: name, BaseURL: baseURL}
	if clientID != "" {
		encoded, err := json.Marshal(clientID)
		if err != nil {
			return fmt.Errorf("encoding client id: %w", err)
		}
		p.Extra = map[string]json.RawMessage{"client_id": encoded}
	}

	profiles, _, err := app.Profiles.List()
	if err != nil {
		return fmt.Errorf("could not read profiles: %w", err)
	}

	if _, exists := profiles[name]; !exists {
		if err := app.Profiles.Create(p); err != nil {
			return fmt.Errorf("could not save the profile: %w", err)
		}
	}
	if err := app.Profiles.SetDefault(name); err != nil {
		return fmt.Errorf("saved the profile but could not make it the default: %w", err)
	}
	return nil
}

func newSetupClaudeCmd() *cobra.Command {
	var force bool

	return withForceFlag(&cobra.Command{
		Use:   "claude",
		Short: "Install the skill for Claude Code",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)

			install, err := installClaudeSkill(force)
			if err != nil {
				return err
			}

			verb := "Installed"
			if install.updated {
				verb = "Updated"
			}

			opts := []output.ResponseOption{
				output.WithSummary(fmt.Sprintf("%s the weeks skill at %s.", verb, install.path)),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "verify", Cmd: "weeks doctor --json", Description: "Confirm the harness can see it",
				}),
			}
			if !harness.DetectClaude() {
				opts = append(opts, output.WithNotice(
					"Claude Code does not appear to be installed here; the skill is in place for when it is."))
			}

			return app.Out.OK(map[string]any{
				"harness": "claude",
				"path":    install.path,
				"updated": install.updated,
			}, opts...)
		},
	}, &force)
}

type skillInstall struct {
	path    string
	updated bool
}

func installClaudeSkill(force bool) (*skillInstall, error) {
	dir := harness.SkillDir()
	if dir == "" {
		return nil, output.ErrUsage("cannot locate a home directory to install into")
	}

	target := filepath.Join(dir, "SKILL.md")
	existed := fileExists(target)
	if existed && !force {
		return nil, &output.Error{
			Code:    output.CodeUsage,
			Message: target + " already exists",
			Hint:    "Pass --force to overwrite it with this binary's version.",
		}
	}

	data, err := skills.FS.ReadFile(SkillPath)
	if err != nil {
		return nil, fmt.Errorf("reading the embedded skill: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return nil, fmt.Errorf("writing %s: %w", target, err)
	}

	return &skillInstall{path: target, updated: existed}, nil
}

func withForceFlag(cmd *cobra.Command, force *bool) *cobra.Command {
	cmd.Flags().BoolVar(force, "force", false, "Overwrite an existing skill file")
	return cmd
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
