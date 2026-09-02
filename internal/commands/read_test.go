package commands

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestResourceListRendersHumanSummary(t *testing.T) {
	list := ResourceList{
		{"id": "space_abc", "name": "Studio", "counts": map[string]any{"plans_count": float64(2), "people_count": float64(5)}},
	}

	var buf bytes.Buffer
	if err := list.RenderStyled(&buf, &output.Style{}); err != nil {
		t.Fatalf("RenderStyled: %v", err)
	}

	got := buf.String()
	for _, want := range []string{"Studio", "space_abc", "people: 5", "plans: 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered %q, missing %q", got, want)
		}
	}

	firstLine := strings.Split(strings.TrimSpace(got), "\n")[0]
	if !strings.HasPrefix(firstLine, "space_abc") {
		t.Fatalf("first line = %q, want id first", firstLine)
	}
}

func TestResourceKeepsImportantKeysFirst(t *testing.T) {
	r := Resource{"updated_at": "later", "name": "Launch", "id": "plan_abc", "space_id": "space_abc"}

	var buf bytes.Buffer
	if err := r.RenderStyled(&buf, &output.Style{}); err != nil {
		t.Fatalf("RenderStyled: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if !strings.Contains(lines[0], "id") || !strings.Contains(lines[1], "name") || !strings.Contains(lines[2], "space id") {
		t.Fatalf("unexpected order: %q", buf.String())
	}
}

func TestPlanSnapshotRendersIncludedCollections(t *testing.T) {
	plan := PlanResource{
		"id":       "plan_abc",
		"name":     "Launch",
		"space_id": "space_abc",
		"people": []any{
			map[string]any{
				"id":           "person_abc",
				"display_name": "Dana",
				"role":         "Producer",
				"space_person": map[string]any{"id": "space_person_abc", "name": "Dana"},
			},
		},
		"jobs": []any{
			map[string]any{"id": "job_abc", "name": "Lighting", "person_id": "person_abc"},
		},
		"slots": []any{
			map[string]any{
				"id":        "slot_abc",
				"starts_at": "2026-09-02T09:00:00Z",
				"job_id":    "job_abc",
				"timeline": map[string]any{
					"label":            "2026-09-02",
					"starts_at":        "2026-09-02T09:00:00Z",
					"ends_at_earliest": "2026-09-02T17:00:00Z",
					"status":           "future",
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := plan.RenderStyled(&buf, &output.Style{}); err != nil {
		t.Fatalf("RenderStyled: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"id        plan_abc",
		"people  1 items",
		"person_abc  Dana",
		"role  Producer",
		"jobs  1 items",
		"job_abc  Lighting",
		"person id  person_abc",
		"slots  1 items",
		"slot_abc",
		"starts at  2026-09-02T09:00:00Z",
		"timeline   2026-09-02, starts 2026-09-02T09:00:00Z, ends 2026-09-02T17:00:00Z, future",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "people      1 items") {
		t.Fatalf("snapshot collection was collapsed into scalar fields:\n%s", got)
	}
	if strings.Contains(got, "space person") || strings.Contains(got, "map[") {
		t.Fatalf("snapshot renderer leaked nested implementation detail:\n%s", got)
	}
}

func TestResourceListRendersTypedTeamIDs(t *testing.T) {
	list := ResourceList{{"id": "team_abc", "name": "Ops"}}

	var buf bytes.Buffer
	if err := list.RenderStyled(&buf, &output.Style{}); err != nil {
		t.Fatalf("RenderStyled: %v", err)
	}

	if got := buf.String(); !strings.HasPrefix(got, "team_abc") || !strings.Contains(got, "Ops") {
		t.Fatalf("rendered %q, want typed id first and name", got)
	}
}

func TestTeamsListCallsAPI(t *testing.T) {
	server, app := readTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/teams" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"team_abc","name":"Ops"}]`))
	})
	defer server.Close()

	cmd := NewTeamsCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := appOutput(app)
	if !strings.Contains(got, "Ops") || !strings.Contains(got, "team_abc") || !strings.Contains(got, "weeks spaces list --team <team-id>") {
		t.Fatalf("output = %q", got)
	}
}

func TestSpacesListDefaultsWhenOneTeamIsAvailable(t *testing.T) {
	var paths []string
	server, app := readTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/teams":
			_, _ = w.Write([]byte(`[{"id":"team_abc","name":"Ops"}]`))
		case "/api/v1/teams/team_abc/spaces":
			_, _ = w.Write([]byte(`[{"id":"space_abc","name":"Studio"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	defer server.Close()

	cmd := NewSpacesCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Join(paths, ",") != "/api/v1/teams,/api/v1/teams/team_abc/spaces" {
		t.Fatalf("paths = %v", paths)
	}
	if got := appOutput(app); !strings.Contains(got, "Studio") || !strings.Contains(got, "space_abc") {
		t.Fatalf("output = %q", got)
	}
}

func TestSpacesListRequiresTeamWhenMultipleTeamsAreAvailable(t *testing.T) {
	server, app := readTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"team_abc","name":"Ops"},{"id":"team_def","name":"Field"}]`))
	})
	defer server.Close()

	cmd := NewSpacesCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if output.AsError(err).Code != output.CodeUsage {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
	if !strings.Contains(err.Error(), "weeks teams list") {
		t.Fatalf("error = %v", err)
	}
}

func TestSpacesListUsesDefaultTeam(t *testing.T) {
	var paths []string
	server, app := readTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/api/v1/teams/team_abc/spaces" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"space_abc","name":"Studio"}]`))
	})
	defer server.Close()
	setReadTestDefaults(t, app, defaults{TeamID: "team_abc"})

	cmd := NewSpacesCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Join(paths, ",") != "/api/v1/teams/team_abc/spaces" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestPlansListUsesDefaultSpace(t *testing.T) {
	server, app := readTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/spaces/space_abc/staffing/plans" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"plan_abc","name":"Launch","space_id":"space_abc"}]`))
	})
	defer server.Close()
	setReadTestDefaults(t, app, defaults{TeamID: "team_abc", SpaceID: "space_abc"})

	cmd := NewPlansCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestReadAuthBreadcrumbsRespectGlobalScope(t *testing.T) {
	err := readErrorNext(&appctx.App{ConfigScope: config.ScopeGlobal, Profile: "acme"}, output.ErrAuth("not signed in"))

	var withCrumbs *output.BreadcrumbError
	if !errors.As(err, &withCrumbs) {
		t.Fatalf("err = %T, want BreadcrumbError", err)
	}
	if len(withCrumbs.Breadcrumbs) != 2 {
		t.Fatalf("breadcrumbs = %#v", withCrumbs.Breadcrumbs)
	}
	if withCrumbs.Breadcrumbs[0].Cmd != "weeks --global auth login --profile acme" {
		t.Fatalf("login breadcrumb = %q", withCrumbs.Breadcrumbs[0].Cmd)
	}
	if withCrumbs.Breadcrumbs[1].Cmd != "weeks --global setup --profile acme" {
		t.Fatalf("setup breadcrumb = %q", withCrumbs.Breadcrumbs[1].Cmd)
	}
}

func TestSelectionBreadcrumbsRespectScopeAndProfile(t *testing.T) {
	local := &appctx.App{ConfigScope: config.ScopeLocal, Profile: "acme"}
	teamCrumbs := teamSelectionBreadcrumbs(local)
	if teamCrumbs[0].Cmd != "weeks defaults set --profile acme" {
		t.Fatalf("local team defaults breadcrumb = %q", teamCrumbs[0].Cmd)
	}
	if teamCrumbs[1].Cmd != "weeks teams list --profile acme" {
		t.Fatalf("local team list breadcrumb = %q", teamCrumbs[1].Cmd)
	}

	global := &appctx.App{ConfigScope: config.ScopeGlobal, Profile: "acme"}
	spaceCrumbs := spaceSelectionBreadcrumbs(global)
	if len(spaceCrumbs) != 1 {
		t.Fatalf("global space breadcrumbs = %#v, want only list", spaceCrumbs)
	}
	if spaceCrumbs[0].Cmd != "weeks --global spaces list --profile acme" {
		t.Fatalf("global space list breadcrumb = %q", spaceCrumbs[0].Cmd)
	}
}

func readTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *appctx.App) {
	t.Helper()
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvConfigDir, t.TempDir())

	server := httptest.NewServer(handler)
	store := auth.NewFileStore(config.Dir())
	if err := store.Save("", &auth.Credentials{AccessToken: "tok", BaseURL: server.URL}); err != nil {
		server.Close()
		t.Fatalf("Save: %v", err)
	}

	var out bytes.Buffer
	app := &appctx.App{
		Out:         output.New(output.Options{Format: output.FormatStyled, Writer: &out}),
		BaseURL:     server.URL,
		ConfigDir:   config.Dir(),
		ConfigScope: config.ScopeEnv,
		Profiles:    bcprofile.NewStore(config.ProfilesPath()),
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("output: %s", out.String())
		}
	})
	readTestOutputs[app] = &out
	return server, app
}

func setReadTestDefaults(t *testing.T, app *appctx.App, current defaults) {
	t.Helper()
	prof := &bcprofile.Profile{Name: "default", BaseURL: app.BaseURL}
	if err := setProfileStringExtra(prof, "default_team_id", current.TeamID); err != nil {
		t.Fatal(err)
	}
	if err := setProfileStringExtra(prof, "default_space_id", current.SpaceID); err != nil {
		t.Fatal(err)
	}
	if err := app.Profiles.Create(prof); err != nil {
		t.Fatal(err)
	}
	app.Profile = "default"
	if creds, err := app.Creds().Load("", app.BaseURL); err == nil {
		if err := app.Creds().Save(app.Profile, creds); err != nil {
			t.Fatal(err)
		}
	}
}

var readTestOutputs = map[*appctx.App]*bytes.Buffer{}

func appOutput(app *appctx.App) string {
	return readTestOutputs[app].String()
}
