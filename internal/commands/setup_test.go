package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestSetupCreatesDefaultProfileAndInstallsSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(config.EnvConfigDir, t.TempDir())

	var out bytes.Buffer
	profiles := bcprofile.NewStore(config.ProfilesPath())
	cmd := NewSetupCmd()
	cmd.SetContext(appctx.With(context.Background(), &appctx.App{
		Out:      output.New(output.Options{Format: output.FormatJSON, Writer: &out}),
		BaseURL:  "https://weeks.example",
		ClientID: "client-123",
		Profiles: profiles,
	}))
	cmd.SetArgs([]string{"--profile", "local"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	gotProfiles, defaultName, err := profiles.List()
	if err != nil {
		t.Fatal(err)
	}
	if defaultName != "local" {
		t.Fatalf("default = %q", defaultName)
	}
	if gotProfiles["local"].BaseURL != "https://weeks.example" {
		t.Fatalf("profile = %#v", gotProfiles["local"])
	}

	skillPath := filepath.Join(home, ".claude", "skills", "weeks", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("skill not installed: %v", err)
	}
	if !strings.Contains(out.String(), "weeks auth login --profile local") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestSetupProtectsExistingSkillWithoutForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(config.EnvConfigDir, t.TempDir())

	skillPath := filepath.Join(home, ".claude", "skills", "weeks", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewSetupCmd()
	cmd.SetContext(appctx.With(context.Background(), &appctx.App{
		Out:      output.New(output.Options{Format: output.FormatJSON, Writer: &bytes.Buffer{}}),
		BaseURL:  config.DefaultBaseURL,
		Profiles: bcprofile.NewStore(config.ProfilesPath()),
	}))
	cmd.SetArgs([]string{"--skip-profile"})

	err := cmd.Execute()
	if output.AsError(err).Code != output.CodeUsage {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
	if got, _ := os.ReadFile(skillPath); string(got) != "mine" {
		t.Fatalf("skill was overwritten: %q", got)
	}
}
