package commands

import (
	"bufio"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// NewDefaultsCmd builds `weeks defaults`.
func NewDefaultsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defaults",
		Short: "Manage this folder's default team and space",
		Long: "Defaults are local to the working folder. They let read commands choose the team or space\n" +
			"without making one agent's folder inherit another folder's login or selection.",
	}
	cmd.AddCommand(newDefaultsShowCmd(), newDefaultsSetCmd(), newDefaultsClearCmd())
	return cmd
}

func newDefaultsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the active default team and space",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			if err := requireLocalDefaults(app); err != nil {
				return err
			}
			current := currentDefaults(app)
			return app.Out.OK(map[string]any{
				"profile":      app.Profile,
				"config_scope": app.ConfigScope,
				"config_dir":   app.ConfigDir,
				"team_id":      emptyNil(current.TeamID),
				"space_id":     emptyNil(current.SpaceID),
			},
				output.WithSummary(defaultsSummary(current)),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "set", Cmd: "weeks defaults set", Description: "Choose defaults for this folder",
				}),
			)
		},
	}
}

func newDefaultsSetCmd() *cobra.Command {
	var teamID, spaceID string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Choose this folder's default team and space",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			if err := requireLocalDefaults(app); err != nil {
				return err
			}

			profileName, err := ensureLocalDefaultsProfile(app)
			if err != nil {
				return err
			}
			app.Profile = profileName
			prof, err := app.Profiles.Get(profileName)
			if err != nil {
				return err
			}

			chosenTeam := strings.TrimSpace(teamID)
			chosenSpace := strings.TrimSpace(spaceID)

			if chosenTeam == "" && app.Interactive {
				teams, err := listTeams(cmd, app)
				if err != nil {
					return err
				}
				chosenTeam, err = chooseResource(cmd, "Team", teams)
				if err != nil {
					return err
				}
			}
			if chosenTeam == "" {
				return output.WithErrorNext(
					output.ErrUsage("--team is required outside interactive setup"),
					output.Breadcrumb{Action: "teams", Cmd: "weeks teams list", Description: "List teams you can access"},
				)
			}

			if chosenSpace == "" && app.Interactive {
				spaces, err := listSpaces(cmd, app, chosenTeam)
				if err != nil {
					return err
				}
				chosenSpace, err = chooseResource(cmd, "Space", spaces)
				if err != nil {
					return err
				}
			}

			if err := setProfileStringExtra(prof, "default_team_id", chosenTeam); err != nil {
				return err
			}
			if err := setProfileStringExtra(prof, "default_space_id", chosenSpace); err != nil {
				return err
			}
			if err := upsertProfile(app.Profiles, prof, true); err != nil {
				return err
			}

			current := profileDefaults(prof)
			return app.Out.OK(map[string]any{
				"profile":      profileName,
				"config_scope": app.ConfigScope,
				"config_dir":   app.ConfigDir,
				"team_id":      current.TeamID,
				"space_id":     emptyNil(current.SpaceID),
			},
				output.WithSummary(defaultsSummary(current)),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "spaces", Cmd: "weeks spaces list", Description: "Use the default team"},
					output.Breadcrumb{Action: "plans", Cmd: "weeks plans list", Description: "Use the default space"},
				),
			)
		},
	}
	cmd.Flags().StringVar(&teamID, "team", "", "Team id to use by default")
	cmd.Flags().StringVar(&spaceID, "space", "", "Space id to use by default")
	return cmd
}

func newDefaultsClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear this folder's default team and space",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			if err := requireLocalDefaults(app); err != nil {
				return err
			}
			if app.Profile == "" {
				return output.ErrUsage("no active local profile has defaults to clear")
			}
			prof, err := app.Profiles.Get(app.Profile)
			if err != nil {
				return err
			}
			if err := setProfileStringExtra(prof, "default_team_id", ""); err != nil {
				return err
			}
			if err := setProfileStringExtra(prof, "default_space_id", ""); err != nil {
				return err
			}
			if err := upsertProfile(app.Profiles, prof, true); err != nil {
				return err
			}
			return app.Out.OK(map[string]any{
				"profile":      app.Profile,
				"config_scope": app.ConfigScope,
				"config_dir":   app.ConfigDir,
			},
				output.WithSummary("Cleared default team and space."),
				output.WithBreadcrumbs(output.Breadcrumb{
					Action: "set", Cmd: "weeks defaults set", Description: "Choose new defaults",
				}),
			)
		},
	}
}

func requireLocalDefaults(app *appctx.App) error {
	if app.ConfigScope == config.ScopeLocal {
		return nil
	}
	return output.WithErrorNext(
		output.ErrUsage("defaults are local to a working folder; run without --global or WEEKS_CONFIG_DIR"),
		output.Breadcrumb{Action: "local", Cmd: "weeks defaults set", Description: "Choose folder-local defaults"},
	)
}

func listSpaces(cmd *cobra.Command, app *appctx.App, teamID string) (ResourceList, error) {
	data, err := apiGetJSON(cmd, app, "/api/v1/teams/"+url.PathEscape(teamID)+"/spaces", nil)
	if err != nil {
		return nil, err
	}
	return resourceList(data), nil
}

func chooseResource(cmd *cobra.Command, label string, items ResourceList) (string, error) {
	if len(items) == 0 {
		return "", output.ErrUsage("no " + strings.ToLower(label) + "s found")
	}
	if len(items) == 1 {
		return idOf(items[0]), nil
	}

	w := cmd.ErrOrStderr()
	_, _ = fmt.Fprintf(w, "\n%s\n", label)
	for i, item := range items {
		_, _ = fmt.Fprintf(w, "  %d. %s  %s\n", i+1, idOf(item), resourceLabel(item))
	}
	_, _ = fmt.Fprintf(w, "Choose %s [1-%d]: ", strings.ToLower(label), len(items))

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return "", err
	}
	index, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || index < 1 || index > len(items) {
		return "", output.ErrUsage("selection must be a number between 1 and " + strconv.Itoa(len(items)))
	}
	return idOf(items[index-1]), nil
}

func defaultsSummary(current defaults) string {
	switch {
	case current.TeamID != "" && current.SpaceID != "":
		return fmt.Sprintf("Default team is %s and default space is %s.", current.TeamID, current.SpaceID)
	case current.TeamID != "":
		return fmt.Sprintf("Default team is %s.", current.TeamID)
	default:
		return "No default team or space is set."
	}
}

func emptyNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
