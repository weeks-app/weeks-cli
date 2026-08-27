package commands

import (
	"encoding/json"
	"fmt"
	"sort"

	bcprofile "github.com/basecamp/cli/profile"
	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// NewProfileCmd builds `weeks profile`.
//
// A profile is one weeks installation plus the credential for it. Because the
// credential store keys on the profile name, `weeks --profile acme` and
// `weeks --profile beta` cannot see each other's tokens — which is how one
// operator drives several teams without one of them ever holding the other's
// access.
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage named profiles, one per installation or team",
		Long: "A profile names a weeks installation and owns its credential.\n\n" +
			"Select one with --profile, or WEEKS_PROFILE, or by making it the default.\n" +
			"Credentials are stored per profile, so profiles are the boundary between teams.",
	}
	cmd.AddCommand(
		newProfileListCmd(),
		newProfileSetCmd(),
		newProfileRemoveCmd(),
		newProfileDefaultCmd(),
	)
	return cmd
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the configured profiles",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)

			profiles, defaultName, err := app.Profiles.List()
			if err != nil {
				return fmt.Errorf("could not read profiles: %w", err)
			}

			names := make([]string, 0, len(profiles))
			for name := range profiles {
				names = append(names, name)
			}
			sort.Strings(names)

			rows := make([]map[string]any, 0, len(names))
			for _, name := range names {
				rows = append(rows, map[string]any{
					"id":         name,
					"name":       name,
					"base_url":   profiles[name].BaseURL,
					"is_default": name == defaultName,
					"active":     name == app.Profile,
				})
			}

			summary := fmt.Sprintf("%d profiles configured.", len(rows))
			crumbs := []output.Breadcrumb{
				{Action: "add", Cmd: "weeks profile set <name> --base-url <url>", Description: "Add or update a profile"},
			}
			if len(rows) == 0 {
				summary = "No profiles configured; commands use " + app.BaseURL + "."
			} else {
				crumbs = append(crumbs, output.Breadcrumb{
					Action: "default", Cmd: "weeks profile default <name>", Description: "Choose the profile used when none is named",
				})
			}

			return app.Out.OK(rows, output.WithSummary(summary), output.WithBreadcrumbs(crumbs...))
		},
	}
}

func newProfileSetCmd() *cobra.Command {
	var (
		baseURL     string
		clientID    string
		makeDefault bool
	)

	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appctx.From(cmd)
			name := args[0]

			if err := bcprofile.ValidateName(name); err != nil {
				return output.ErrUsage(err.Error())
			}
			if baseURL == "" {
				return output.ErrUsage("--base-url is required: a profile names an installation")
			}

			p := &bcprofile.Profile{Name: name, BaseURL: baseURL}
			if clientID != "" {
				encoded, err := json.Marshal(clientID)
				if err != nil {
					return fmt.Errorf("encoding client id: %w", err)
				}
				p.Extra = map[string]json.RawMessage{"client_id": encoded}
			}

			if err := app.Profiles.Create(p); err != nil {
				return fmt.Errorf("could not save the profile: %w", err)
			}
			if makeDefault {
				if err := app.Profiles.SetDefault(name); err != nil {
					return fmt.Errorf("saved the profile but could not make it the default: %w", err)
				}
			}

			return app.Out.OK(map[string]any{
				"id":         name,
				"name":       name,
				"base_url":   baseURL,
				"client_id":  clientID,
				"is_default": makeDefault,
			},
				output.WithSummary(fmt.Sprintf("Profile %s points at %s.", name, baseURL)),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "login", Cmd: "weeks auth login --profile " + name, Description: "Sign in as this profile",
				}),
			)
		},
	}

	cmd.Flags().StringVar(&baseURL, "base-url", "", "The weeks installation this profile targets (required)")
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client id for this installation")
	cmd.Flags().BoolVar(&makeDefault, "default", false, "Also make this the default profile")
	return cmd
}

func newProfileRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a profile",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appctx.From(cmd)
			name := args[0]

			profiles, _, err := app.Profiles.List()
			if err != nil {
				return fmt.Errorf("could not read profiles: %w", err)
			}
			prof, ok := profiles[name]
			if !ok {
				return output.ErrNotFound("profile", name)
			}

			// Removing the profile without removing its credential would leave
			// a token in the keyring that nothing can name any more.
			_ = app.Creds.Delete(name, prof.BaseURL)

			if err := app.Profiles.Delete(name); err != nil {
				return fmt.Errorf("could not remove the profile: %w", err)
			}

			return app.Out.OK(map[string]any{"id": name, "name": name},
				output.WithSummary(fmt.Sprintf("Removed profile %s and its stored credential.", name)),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "list", Cmd: "weeks profile list", Description: "See what is left",
				}),
			)
		},
	}
}

func newProfileDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <name>",
		Short: "Choose the profile used when none is named",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appctx.From(cmd)
			name := args[0]

			if err := app.Profiles.SetDefault(name); err != nil {
				return output.ErrNotFound("profile", name)
			}

			return app.Out.OK(map[string]any{"id": name, "name": name, "is_default": true},
				output.WithSummary(fmt.Sprintf("Profile %s is now the default.", name)),
			)
		},
	}
}
