package commands

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/weeks-app/weeks-cli/internal/api"
	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// Resource is one API object. It renders compactly for a human while still
// encoding as the object returned by the server in JSON modes.
type Resource map[string]any

// ResourceList is an API collection. It renders as a table-ish list for a
// human while still encoding as the server's array in JSON modes.
type ResourceList []Resource

// NewTeamsCmd builds `weeks teams`.
func NewTeamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "teams",
		Short: "List and view teams",
		Long:  "Teams own spaces. List them first when you do not know which team id to use.",
	}
	cmd.AddCommand(newTeamsListCmd(), newTeamsViewCmd())
	return cmd
}

func newTeamsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List teams you can access",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			teams, err := listTeams(cmd, app)
			if err != nil {
				return err
			}
			return app.Out.OK(teams,
				output.WithSummary(fmt.Sprintf("%d teams.", len(teams))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "view", Cmd: "weeks teams view <team-id>", Description: "Fetch one team"},
					output.Breadcrumb{Action: "spaces", Cmd: "weeks spaces list --team <team-id>", Description: "List spaces inside a team"},
				),
			)
		},
	}
	return cmd
}

func newTeamsViewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view <team-id>",
		Short: "Fetch one team",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appctx.From(cmd)
			data, err := apiGetJSON(cmd, app, "/api/v1/teams/"+url.PathEscape(args[0]), nil)
			if err != nil {
				return err
			}
			team := resource(data)
			return app.Out.OK(team,
				output.WithSummary(fmt.Sprintf("Team %s.", resourceLabel(team))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "spaces", Cmd: "weeks spaces list --team " + idOf(team), Description: "List spaces inside this team"},
				),
			)
		},
	}
	return cmd
}

// NewSpacesCmd builds `weeks spaces`.
func NewSpacesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spaces",
		Short: "List and view spaces",
		Long:  "Spaces are the top-level containers a team plans inside.",
	}
	cmd.AddCommand(newSpacesListCmd(), newSpacesViewCmd())
	return cmd
}

func newSpacesListCmd() *cobra.Command {
	var teamID string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List spaces in a team",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			resolvedTeamID, err := resolveTeamID(cmd, app, teamID)
			if err != nil {
				if output.AsError(err).Code == output.CodeAuth {
					return err
				}
				return output.WithErrorNext(err, teamSelectionBreadcrumbs(app)...)
			}

			data, err := apiGetJSON(cmd, app, "/api/v1/teams/"+url.PathEscape(resolvedTeamID)+"/spaces", nil)
			if err != nil {
				return err
			}
			spaces := resourceList(data)
			return app.Out.OK(spaces,
				output.WithSummary(fmt.Sprintf("%d spaces.", len(spaces))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "view", Cmd: "weeks spaces view <space-id>", Description: "Fetch one space"},
					output.Breadcrumb{Action: "plans", Cmd: "weeks plans list --space <space-id>", Description: "List plans inside a space"},
				),
			)
		},
	}
	cmd.Flags().StringVar(&teamID, "team", "", "Team id whose spaces should be listed; defaults when you can access exactly one team")
	return cmd
}

func newSpacesViewCmd() *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:   "view <space-id>",
		Short: "Fetch one space",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appctx.From(cmd)
			query := includeQuery(include)
			data, err := apiGetJSON(cmd, app, "/api/v1/spaces/"+url.PathEscape(args[0]), query)
			if err != nil {
				return err
			}
			space := resource(data)
			return app.Out.OK(space,
				output.WithSummary(fmt.Sprintf("Space %s.", resourceLabel(space))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "plans", Cmd: "weeks plans list --space " + idOf(space), Description: "List plans inside this space"},
					output.Breadcrumb{Action: "overview", Cmd: "weeks spaces view " + idOf(space) + " --include overview", Description: "Fetch people, plans, and counts"},
				),
			)
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "Public include scope to ask the API for, such as counts, plans, or overview")
	return cmd
}

// NewPlansCmd builds `weeks plans`.
func NewPlansCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plans",
		Short: "List and view staffing plans",
		Long:  "Plans are schedules inside a space: productions, events, or periods.",
	}
	cmd.AddCommand(newPlansListCmd(), newPlansViewCmd())
	return cmd
}

func newPlansListCmd() *cobra.Command {
	var spaceID string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List plans in a space",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appctx.From(cmd)
			if spaceID == "" {
				spaceID = currentDefaults(app).SpaceID
			}
			if spaceID == "" {
				return output.WithErrorNext(
					output.ErrUsage("--space is required"),
					spaceSelectionBreadcrumbs(app)...,
				)
			}

			data, err := apiGetJSON(cmd, app, "/api/v1/spaces/"+url.PathEscape(spaceID)+"/staffing/plans", nil)
			if err != nil {
				return err
			}
			plans := resourceList(data)
			return app.Out.OK(plans,
				output.WithSummary(fmt.Sprintf("%d plans.", len(plans))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "view", Cmd: "weeks plans view <plan-id>", Description: "Fetch one plan"},
					output.Breadcrumb{Action: "snapshot", Cmd: "weeks plans view <plan-id> --include snapshot", Description: "Fetch the full planning snapshot"},
				),
			)
		},
	}
	cmd.Flags().StringVar(&spaceID, "space", "", "Space id whose plans should be listed (required)")
	return cmd
}

func newPlansViewCmd() *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:   "view <plan-id>",
		Short: "Fetch one staffing plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appctx.From(cmd)
			data, err := apiGetJSON(cmd, app, "/api/v1/staffing/plans/"+url.PathEscape(args[0]), includeQuery(include))
			if err != nil {
				return err
			}
			plan := resource(data)
			return app.Out.OK(plan,
				output.WithSummary(fmt.Sprintf("Plan %s.", resourceLabel(plan))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "space", Cmd: "weeks spaces view " + stringValue(plan["space_id"]), Description: "Fetch the space this plan belongs to"},
					output.Breadcrumb{Action: "snapshot", Cmd: "weeks plans view " + idOf(plan) + " --include snapshot", Description: "Fetch people, jobs, slots, and assignments"},
				),
			)
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "Public include scope to ask the API for, such as counts, slots, or snapshot")
	return cmd
}

func apiClient(app *appctx.App) *api.Client {
	return &api.Client{BaseURL: app.BaseURL, Profile: app.Profile, Creds: app.Creds()}
}

func apiGetJSON(cmd *cobra.Command, app *appctx.App, path string, query url.Values) (any, error) {
	data, err := apiClient(app).GetJSON(cmd.Context(), path, query)
	if err != nil {
		return nil, readErrorNext(app, err)
	}
	return data, nil
}

func readErrorNext(app *appctx.App, err error) error {
	if output.AsError(err).Code != output.CodeAuth {
		return err
	}
	description := "Sign in to this folder"
	if app.ConfigScope == config.ScopeGlobal {
		description = "Sign in to global storage"
	}
	return output.WithErrorNext(err,
		output.Breadcrumb{Action: "login", Cmd: scopedCommand(app, "weeks auth login"), Description: description},
		output.Breadcrumb{Action: "setup", Cmd: scopedCommand(app, "weeks setup"), Description: "Check config and agent setup"},
	)
}

func teamSelectionBreadcrumbs(app *appctx.App) []output.Breadcrumb {
	crumbs := []output.Breadcrumb{}
	if app.ConfigScope == config.ScopeLocal {
		crumbs = append(crumbs, output.Breadcrumb{Action: "defaults", Cmd: "weeks defaults set", Description: "Choose a default team for this folder"})
	}
	return append(crumbs, output.Breadcrumb{Action: "teams", Cmd: scopedCommand(app, "weeks teams list"), Description: "List teams you can access"})
}

func spaceSelectionBreadcrumbs(app *appctx.App) []output.Breadcrumb {
	crumbs := []output.Breadcrumb{}
	if app.ConfigScope == config.ScopeLocal {
		crumbs = append(crumbs, output.Breadcrumb{Action: "defaults", Cmd: "weeks defaults set", Description: "Choose a default space for this folder"})
	}
	return append(crumbs, output.Breadcrumb{Action: "spaces", Cmd: scopedCommand(app, "weeks spaces list"), Description: "List spaces you can access"})
}

func listTeams(cmd *cobra.Command, app *appctx.App) (ResourceList, error) {
	data, err := apiGetJSON(cmd, app, "/api/v1/teams", nil)
	if err != nil {
		return nil, err
	}
	return resourceList(data), nil
}

func resolveTeamID(cmd *cobra.Command, app *appctx.App, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if teamID := currentDefaults(app).TeamID; teamID != "" {
		return teamID, nil
	}

	teams, err := listTeams(cmd, app)
	if err != nil {
		return "", err
	}
	switch len(teams) {
	case 0:
		return "", output.ErrUsage("no teams found for this profile")
	case 1:
		return idOf(teams[0]), nil
	default:
		return "", output.ErrUsage("--team is required when this profile can access multiple teams; run `weeks teams list`")
	}
}

func includeQuery(include string) url.Values {
	query := url.Values{}
	if include != "" {
		query.Set("include", include)
	}
	return query
}

func resource(data any) Resource {
	if item, ok := data.(map[string]any); ok {
		return Resource(item)
	}
	return Resource{"value": data}
}

func resourceList(data any) ResourceList {
	items, ok := data.([]any)
	if !ok {
		return ResourceList{resource(data)}
	}
	resources := make(ResourceList, 0, len(items))
	for _, item := range items {
		resources = append(resources, resource(item))
	}
	return resources
}

func (r Resource) RenderStyled(w io.Writer, style *output.Style) error {
	keys := orderedKeys(r)
	width := 0
	for _, key := range keys {
		if len(key) > width {
			width = len(key)
		}
	}
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s%-*s%s  %s\n", style.Dim, width, strings.ReplaceAll(key, "_", " "), style.Reset, styledValue(r[key])); err != nil {
			return err
		}
	}
	return nil
}

func (l ResourceList) RenderStyled(w io.Writer, style *output.Style) error {
	for _, item := range l {
		if _, err := fmt.Fprintf(w, "%s%s%s  %s\n", style.Bold, idOf(item), style.Reset, resourceLabel(item)); err != nil {
			return err
		}
		if counts, ok := item["counts"].(map[string]any); ok && len(counts) > 0 {
			if _, err := fmt.Fprintf(w, "  %s%s%s\n", style.Dim, summarizeCounts(counts), style.Reset); err != nil {
				return err
			}
		}
	}
	return nil
}

func orderedKeys(r Resource) []string {
	priority := []string{"id", "name", "space_id", "team_id", "time_zone", "locale", "archived_at", "counts", "created_at", "updated_at"}
	seen := map[string]bool{}
	var keys []string
	for _, key := range priority {
		if _, ok := r[key]; ok {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range r {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

func resourceLabel(r Resource) string {
	if name := stringValue(r["name"]); name != "" {
		return name
	}
	return idOf(r)
}

func idOf(r Resource) string {
	return stringValue(r["id"])
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	switch typed := value.(type) {
	case int:
		return fmt.Sprint(typed)
	case int64:
		return fmt.Sprint(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%.0f", typed)
		}
		return fmt.Sprint(typed)
	}
	return ""
}

func styledValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "-"
	case string:
		if typed == "" {
			return "-"
		}
		return typed
	case map[string]any:
		return summarizeCounts(typed)
	case []any:
		return fmt.Sprintf("%d items", len(typed))
	default:
		return fmt.Sprint(value)
	}
}

func summarizeCounts(counts map[string]any) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s: %v", strings.TrimSuffix(key, "_count"), counts[key]))
	}
	return strings.Join(parts, ", ")
}
