package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestProfileDefaultReportsConfigReadErrors(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewProfileCmd()
	cmd.SetContext(appctx.With(context.Background(), &appctx.App{
		Out:      output.New(output.Options{Format: output.FormatJSON, Writer: &out}),
		Profiles: bcprofile.NewStore(configPath),
	}))
	cmd.SetArgs([]string{"default", "local"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("profile default succeeded with malformed config")
	}
	if !strings.Contains(err.Error(), "could not read profiles") {
		t.Fatalf("error = %q, want config read error", err)
	}
	if strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %q, should not report not_found for malformed config", err)
	}
}

func TestProfileRemoveDeletesNormalizedHostedCredential(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvNoKeyring, "1")

	profiles := bcprofile.NewStore(config.ProfilesPath())
	if err := profiles.Create(&bcprofile.Profile{Name: "hosted", BaseURL: "https://weeks.app/account"}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	app := &appctx.App{
		Out:      output.New(output.Options{Format: output.FormatJSON, Writer: &out}),
		Profile:  "hosted",
		BaseURL:  config.DefaultBaseURL,
		Profiles: profiles,
	}
	if err := app.Creds().Save("hosted", &auth.Credentials{
		AccessToken: "token",
		BaseURL:     config.DefaultBaseURL,
		CreatedAt:   time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	cmd := NewProfileCmd()
	cmd.SetContext(appctx.With(context.Background(), app))
	cmd.SetArgs([]string{"remove", "hosted"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := app.Creds().Load("hosted", config.DefaultBaseURL); !auth.IsNotFound(err) {
		t.Fatalf("credential load error = %v, want not found", err)
	}
}
