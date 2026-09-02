package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestDefaultsSetCreatesLocalProfileAndMigratesCredential(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvConfigDir, t.TempDir())

	baseURL := "https://weeks.example"
	configDir := config.Dir()
	creds := auth.NewFileStore(configDir)
	if err := creds.Save("", &auth.Credentials{AccessToken: "tok", BaseURL: baseURL}); err != nil {
		t.Fatal(err)
	}

	profiles := bcprofile.NewStore(config.ProfilesPathIn(configDir))
	var out bytes.Buffer
	app := &appctx.App{
		Out:         output.New(output.Options{Format: output.FormatJSON, Writer: &out}),
		BaseURL:     baseURL,
		ConfigDir:   configDir,
		ConfigScope: config.ScopeLocal,
		Profiles:    profiles,
	}

	cmd := NewDefaultsCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"set", "--team", "team_abc", "--space", "space_abc"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	gotProfiles, defaultName, err := profiles.List()
	if err != nil {
		t.Fatal(err)
	}
	if defaultName != "default" {
		t.Fatalf("default profile = %q", defaultName)
	}
	if got := profileDefaults(gotProfiles["default"]); got.TeamID != "team_abc" || got.SpaceID != "space_abc" {
		t.Fatalf("defaults = %#v", got)
	}
	if _, err := creds.Load("default", baseURL); err != nil {
		t.Fatalf("profile credential was not migrated: %v", err)
	}
	if _, err := creds.Load("", baseURL); !auth.IsNotFound(err) {
		t.Fatalf("profile-less credential remains, err = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output was not JSON: %v", err)
	}
	if !strings.Contains(got["summary"].(string), "team_abc") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestDefaultsSetRejectsGlobalScope(t *testing.T) {
	cmd := NewDefaultsCmd()
	cmd.SetContext(appctx.With(context.Background(), &appctx.App{
		Out:         output.New(output.Options{Format: output.FormatJSON, Writer: &bytes.Buffer{}}),
		BaseURL:     "https://weeks.example",
		ConfigScope: config.ScopeGlobal,
		Profiles:    bcprofile.NewStore(config.ProfilesPathIn(t.TempDir())),
	}))
	cmd.SetArgs([]string{"set", "--team", "team_abc"})

	err := cmd.Execute()
	if output.AsError(err).Code != output.CodeUsage {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
	if !strings.Contains(err.Error(), "local") {
		t.Fatalf("error = %v", err)
	}
}

func TestDefaultsShowRejectsGlobalScope(t *testing.T) {
	cmd := NewDefaultsCmd()
	cmd.SetContext(appctx.With(context.Background(), &appctx.App{
		Out:         output.New(output.Options{Format: output.FormatJSON, Writer: &bytes.Buffer{}}),
		BaseURL:     "https://weeks.example",
		ConfigScope: config.ScopeGlobal,
		Profiles:    bcprofile.NewStore(config.ProfilesPathIn(t.TempDir())),
	}))
	cmd.SetArgs([]string{"show"})

	err := cmd.Execute()
	if output.AsError(err).Code != output.CodeUsage {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
}

func TestDefaultsCommandPreservesProfileWithoutGlobalScope(t *testing.T) {
	got := defaultsCommand(&appctx.App{ConfigScope: config.ScopeGlobal, Profile: "acme"}, "weeks defaults set")
	if got != "weeks defaults set --profile acme" {
		t.Fatalf("command = %q", got)
	}
}
