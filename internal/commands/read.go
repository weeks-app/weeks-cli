package commands

import (
	"fmt"
	"io"
	"net/url"
	"reflect"
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

// PlanResource renders included snapshot data as sections instead of reducing
// every array to "N items".
type PlanResource Resource

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
					output.Breadcrumb{Action: "view", Cmd: profileCommand(app, "weeks teams view <team-id>", app.Profile), Description: "Fetch one team"},
					output.Breadcrumb{Action: "spaces", Cmd: profileCommand(app, "weeks spaces list --team <team-id>", app.Profile), Description: "List spaces inside a team"},
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
					output.Breadcrumb{Action: "spaces", Cmd: profileCommand(app, "weeks spaces list --team "+idOf(team), app.Profile), Description: "List spaces inside this team"},
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
					output.Breadcrumb{Action: "view", Cmd: profileCommand(app, "weeks spaces view <space-id>", app.Profile), Description: "Fetch one space"},
					output.Breadcrumb{Action: "plans", Cmd: profileCommand(app, "weeks plans list --space <space-id>", app.Profile), Description: "List plans inside a space"},
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
					output.Breadcrumb{Action: "plans", Cmd: profileCommand(app, "weeks plans list --space "+idOf(space), app.Profile), Description: "List plans inside this space"},
					output.Breadcrumb{Action: "overview", Cmd: profileCommand(app, "weeks spaces view "+idOf(space)+" --include overview", app.Profile), Description: "Fetch people, plans, and counts"},
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
					output.Breadcrumb{Action: "view", Cmd: profileCommand(app, "weeks plans view <plan-id>", app.Profile), Description: "Fetch one plan"},
					output.Breadcrumb{Action: "snapshot", Cmd: profileCommand(app, "weeks plans view <plan-id> --include snapshot", app.Profile), Description: "Fetch the full planning snapshot"},
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
			plan := PlanResource(resource(data))
			resourcePlan := Resource(plan)
			return app.Out.OK(plan,
				output.WithSummary(fmt.Sprintf("Plan %s.", resourceLabel(resourcePlan))),
				output.WithBreadcrumbs(
					output.Breadcrumb{Action: "space", Cmd: profileCommand(app, "weeks spaces view "+stringValue(resourcePlan["space_id"]), app.Profile), Description: "Fetch the space this plan belongs to"},
					output.Breadcrumb{Action: "snapshot", Cmd: profileCommand(app, "weeks plans view "+idOf(resourcePlan)+" --include snapshot", app.Profile), Description: "Fetch people, jobs, slots, and assignments"},
				),
			)
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "Public include scope to ask the API for, such as counts, slots, or snapshot")
	return cmd
}

func apiClient(app *appctx.App) *api.Client {
	return &api.Client{BaseURL: app.BaseURL, Profile: app.Profile, ClientID: app.ClientID, Creds: app.Creds()}
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
		output.Breadcrumb{Action: "login", Cmd: profileCommand(app, "weeks auth login", app.Profile), Description: description},
		output.Breadcrumb{Action: "setup", Cmd: profileCommand(app, "weeks setup", app.Profile), Description: "Check config and agent setup"},
	)
}

func teamSelectionBreadcrumbs(app *appctx.App) []output.Breadcrumb {
	crumbs := []output.Breadcrumb{}
	if app.ConfigScope == config.ScopeLocal {
		crumbs = append(crumbs, output.Breadcrumb{Action: "defaults", Cmd: profileCommand(app, "weeks defaults set", app.Profile), Description: "Choose a default team for this folder"})
	}
	return append(crumbs, output.Breadcrumb{Action: "teams", Cmd: profileCommand(app, "weeks teams list", app.Profile), Description: "List teams you can access"})
}

func spaceSelectionBreadcrumbs(app *appctx.App) []output.Breadcrumb {
	crumbs := []output.Breadcrumb{}
	if app.ConfigScope == config.ScopeLocal {
		crumbs = append(crumbs, output.Breadcrumb{Action: "defaults", Cmd: profileCommand(app, "weeks defaults set", app.Profile), Description: "Choose a default space for this folder"})
	}
	return append(crumbs, output.Breadcrumb{Action: "spaces", Cmd: profileCommand(app, "weeks spaces list", app.Profile), Description: "List spaces you can access"})
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
	return renderScalarFields(w, style, r, orderedKeys(r))
}

func (p PlanResource) RenderStyled(w io.Writer, style *output.Style) error {
	plan := Resource(p)
	if err := renderScalarFields(w, style, plan, orderedScalarKeys(plan)); err != nil {
		return err
	}

	for _, key := range orderedSnapshotSections(plan) {
		items, ok := plan[key].([]any)
		if !ok {
			continue
		}
		if err := renderSnapshotSection(w, style, plan, key, items); err != nil {
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

func renderScalarFields(w io.Writer, style *output.Style, r Resource, keys []string) error {
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

func orderedScalarKeys(r Resource) []string {
	keys := orderedKeys(r)
	scalars := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := r[key].([]any); ok {
			continue
		}
		scalars = append(scalars, key)
	}
	return scalars
}

func orderedSnapshotSections(r Resource) []string {
	priority := []string{"slots", "people", "jobs", "assignments", "routes", "inboxes"}
	seen := map[string]bool{}
	sections := make([]string, 0, len(priority))
	for _, key := range priority {
		if _, ok := r[key].([]any); ok {
			sections = append(sections, key)
			seen[key] = true
		}
	}

	var rest []string
	for key, value := range r {
		if seen[key] {
			continue
		}
		if _, ok := value.([]any); ok {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	return append(sections, rest...)
}

func renderSnapshotSection(w io.Writer, style *output.Style, plan Resource, key string, items []any) error {
	if key == "slots" {
		return renderSlotsSection(w, style, items, newPlanLookup(plan))
	}

	if _, err := fmt.Fprintf(w, "\n%s%s%s  %d items\n", style.Bold, strings.ReplaceAll(key, "_", " "), style.Reset, len(items)); err != nil {
		return err
	}
	for _, raw := range items {
		if err := renderSnapshotItem(w, style, resource(raw)); err != nil {
			return err
		}
	}
	return nil
}

func renderSnapshotItem(w io.Writer, style *output.Style, item Resource) error {
	title := resourceLabel(item)
	id := idOf(item)
	if id != "" && title != id {
		title = id + "  " + title
	}
	if title == "" {
		title = styledValue(item)
	}
	if _, err := fmt.Fprintf(w, "  %s%s%s\n", style.Bold, title, style.Reset); err != nil {
		return err
	}

	keys := orderedScalarKeys(item)
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if skipSnapshotField(key) {
			continue
		}
		if isEmptySnapshotValue(item[key]) {
			continue
		}
		filtered = append(filtered, key)
	}
	return renderIndentedScalarFields(w, style, item, filtered, "    ")
}

type planLookup struct {
	jobs       map[string]string
	people     map[string]string
	slotPeople map[string]string
}

func newPlanLookup(plan Resource) planLookup {
	lookup := planLookup{
		jobs:       map[string]string{},
		people:     map[string]string{},
		slotPeople: map[string]string{},
	}
	for _, raw := range anySlice(plan["jobs"]) {
		job := resource(raw)
		if id := idOf(job); id != "" {
			lookup.jobs[id] = resourceLabel(job)
		}
	}
	for _, raw := range anySlice(plan["people"]) {
		person := resource(raw)
		if id := idOf(person); id != "" {
			lookup.people[id] = resourceLabel(person)
		}
	}
	for _, rawSlot := range anySlice(plan["slots"]) {
		slot := resource(rawSlot)
		for _, rawAssigned := range anySlice(slot["assigned_people"]) {
			assigned := resource(rawAssigned)
			slotPersonID := idOf(assigned)
			personID := stringValue(assigned["person_id"])
			if slotPersonID != "" && personID != "" {
				lookup.slotPeople[slotPersonID] = lookup.personName(personID)
			}
		}
	}
	return lookup
}

func (l planLookup) jobName(id string) string {
	if name := l.jobs[id]; name != "" {
		return name
	}
	return id
}

func (l planLookup) personName(id string) string {
	if name := l.people[id]; name != "" {
		return name
	}
	if name := l.slotPeople[id]; name != "" {
		return name
	}
	return id
}

func renderSlotsSection(w io.Writer, style *output.Style, items []any, lookup planLookup) error {
	if _, err := fmt.Fprintf(w, "\n%sschedule%s  %d slots\n", style.Bold, style.Reset, len(items)); err != nil {
		return err
	}
	for _, raw := range items {
		if err := renderSlot(w, style, resource(raw), lookup); err != nil {
			return err
		}
	}
	return nil
}

func renderSlot(w io.Writer, style *output.Style, slot Resource, lookup planLookup) error {
	title := resourceLabel(slot)
	if id := idOf(slot); id != "" && title != id {
		title = id + "  " + title
	}
	if _, err := fmt.Fprintf(w, "  %s%s%s\n", style.Bold, title, style.Reset); err != nil {
		return err
	}
	if timeline, ok := slot["timeline"].(map[string]any); ok {
		if _, err := fmt.Fprintf(w, "    %swhen%s    %s\n", style.Dim, style.Reset, summarizeMap(timeline)); err != nil {
			return err
		}
	}
	if people := slotPeople(slot, lookup); len(people) > 0 {
		if _, err := fmt.Fprintf(w, "    %speople%s  %s\n", style.Dim, style.Reset, strings.Join(people, ", ")); err != nil {
			return err
		}
	}
	if place := stringValue(slot["place"]); place != "" {
		if _, err := fmt.Fprintf(w, "    %splace%s   %s\n", style.Dim, style.Reset, place); err != nil {
			return err
		}
	}
	if jobs := anySlice(slot["assigned_jobs"]); len(jobs) > 0 {
		if _, err := fmt.Fprintf(w, "    %sjobs%s\n", style.Dim, style.Reset); err != nil {
			return err
		}
		for _, rawJob := range jobs {
			if err := renderSlotJob(w, style, resource(rawJob), lookup); err != nil {
				return err
			}
		}
	}
	return nil
}

func slotPeople(slot Resource, lookup planLookup) []string {
	seen := map[string]bool{}
	var people []string
	for _, id := range stringSlice(slot["person_ids"]) {
		name := lookup.personName(id)
		key := "person:" + id
		if name != "" && !seen[key] {
			people = append(people, name)
			seen[key] = true
		}
	}
	for _, raw := range anySlice(slot["assigned_people"]) {
		assigned := resource(raw)
		personID := stringValue(assigned["person_id"])
		slotPersonID := idOf(assigned)
		name := lookup.personName(personID)
		key := "person:" + personID
		if personID == "" {
			name = lookup.personName(slotPersonID)
			key = "slot_person:" + slotPersonID
		}
		if name != "" && key != "" && !seen[key] {
			people = append(people, name)
			seen[key] = true
		}
	}
	return people
}

func renderSlotJob(w io.Writer, style *output.Style, job Resource, lookup planLookup) error {
	jobID := stringValue(job["job_id"])
	title := lookup.jobName(jobID)
	if jobID != "" && title != jobID {
		title = jobID + "  " + title
	}
	detail := slotJobDetail(job, lookup)
	if detail != "" {
		title += "  " + detail
	}
	if _, err := fmt.Fprintf(w, "      %s\n", title); err != nil {
		return err
	}
	return nil
}

func slotJobDetail(job Resource, lookup planLookup) string {
	var parts []string
	if timeline, ok := job["timeline"].(map[string]any); ok {
		parts = append(parts, summarizeMap(timeline))
	}
	if target := stringValue(job["staffing_requirement_target"]); target != "" {
		parts = append(parts, "target "+target)
	}
	if variance := stringValue(job["staffing_requirement_variance"]); variance != "" {
		parts = append(parts, "variance "+variance)
	}
	names := slotJobPeople(job, lookup)
	if len(names) > 0 {
		parts = append(parts, strings.Join(names, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, "; ") + ")"
}

func slotJobPeople(job Resource, lookup planLookup) []string {
	seen := map[string]bool{}
	var names []string
	assigned := anySlice(job["people"])
	for _, raw := range assigned {
		person := resource(raw)
		assignedPersonID := stringValue(person["assigned_person_id"])
		name := lookup.personName(assignedPersonID)
		if name == "" {
			continue
		}
		if status := stringValue(person["participation_status"]); status != "" {
			name += " " + status
		}
		if comment := stringValue(person["comment"]); comment != "" {
			name += " - " + comment
		}
		if name != "" && !seen[assignedPersonID] {
			names = append(names, name)
			seen[assignedPersonID] = true
		}
	}
	if len(assigned) > 0 {
		return names
	}
	for _, id := range stringSlice(job["assigned_person_ids"]) {
		name := lookup.personName(id)
		if name != "" && !seen[id] {
			names = append(names, name)
			seen[id] = true
		}
	}
	return names
}

func skipSnapshotField(key string) bool {
	switch key {
	case "id", "name", "display_name", "created_at", "updated_at", "plan_id", "space_person", "space_person_id", "reconciliation_status":
		return true
	default:
		return false
	}
}

func isEmptySnapshotValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0 || summarizeMap(typed) == ""
	default:
		value := reflect.ValueOf(value)
		if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
			return value.Len() == 0
		}
		return false
	}
}

func renderIndentedScalarFields(w io.Writer, style *output.Style, r Resource, keys []string, pad string) error {
	width := 0
	for _, key := range keys {
		if len(key) > width {
			width = len(key)
		}
	}
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s%s%-*s%s  %s\n", pad, style.Dim, width, strings.ReplaceAll(key, "_", " "), style.Reset, styledValue(r[key])); err != nil {
			return err
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
	if name := stringValue(r["display_name"]); name != "" {
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

func anySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	default:
		return nil
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := stringValue(item); value != "" {
				values = append(values, value)
			}
		}
		return values
	default:
		return nil
	}
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
		return summarizeMap(typed)
	case []any:
		return fmt.Sprintf("%d items", len(typed))
	default:
		return fmt.Sprint(value)
	}
}

func summarizeMap(values map[string]any) string {
	if label := stringValue(values["label"]); label != "" {
		parts := []string{label}
		if duration := durationSummary(values["duration"]); duration != "" {
			parts = append(parts, duration)
		}
		if status := stringValue(values["status"]); status != "" {
			parts = append(parts, status)
		}
		return strings.Join(parts, ", ")
	}
	return summarizeCounts(values)
}

func durationSummary(value any) string {
	values, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	minimum := stringValue(values["minimum"])
	maximum := stringValue(values["maximum"])
	switch {
	case minimum == "" && maximum == "":
		return ""
	case minimum == maximum:
		return minimum
	case minimum == "":
		return maximum
	case maximum == "":
		return minimum
	default:
		return minimum + "-" + maximum
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
