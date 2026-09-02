package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/config"
)

func TestBuildAppUsesProfileClientID(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvProfile, "")

	store := bcprofile.NewStore(config.ProfilesPath())
	clientID, err := json.Marshal("local-client")
	if err != nil {
		t.Fatal(err)
	}
	createErr := store.Create(&bcprofile.Profile{
		Name:    "local",
		BaseURL: "http://localhost:3000",
		Extra:   map[string]json.RawMessage{"client_id": clientID},
	})
	if createErr != nil {
		t.Fatal(createErr)
	}

	app, err := buildApp(&rootFlags{})
	if err != nil {
		t.Fatal(err)
	}

	if app.ClientID != "local-client" {
		t.Fatalf("ClientID = %q, want local-client", app.ClientID)
	}
}

func TestBuildAppUsesLocalConfigByDefault(t *testing.T) {
	t.Setenv(config.EnvConfigDir, "")
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvProfile, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)

	localDir := filepath.Join(dir, ".weeks")
	store := bcprofile.NewStore(config.ProfilesPathIn(localDir))
	if err := store.Create(&bcprofile.Profile{Name: "local", BaseURL: "https://local.example"}); err != nil {
		t.Fatal(err)
	}

	app, err := buildApp(&rootFlags{})
	if err != nil {
		t.Fatal(err)
	}

	if app.Profile != "local" {
		t.Fatalf("Profile = %q, want local", app.Profile)
	}
	if app.BaseURL != "https://local.example" {
		t.Fatalf("BaseURL = %q, want local profile URL", app.BaseURL)
	}
	if app.ConfigScope != config.ScopeLocal {
		t.Fatalf("ConfigScope = %q, want local", app.ConfigScope)
	}
	if app.ConfigDir != localDir {
		t.Fatalf("ConfigDir = %q, want %q", app.ConfigDir, localDir)
	}
}

func TestBuildAppGlobalFlagUsesGlobalConfig(t *testing.T) {
	t.Setenv(config.EnvConfigDir, "")
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvProfile, "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Chdir(t.TempDir())

	globalDir := filepath.Join(xdg, "weeks")
	store := bcprofile.NewStore(config.ProfilesPathIn(globalDir))
	if err := store.Create(&bcprofile.Profile{Name: "global", BaseURL: "https://global.example"}); err != nil {
		t.Fatal(err)
	}

	app, err := buildApp(&rootFlags{global: true})
	if err != nil {
		t.Fatal(err)
	}

	if app.Profile != "global" {
		t.Fatalf("Profile = %q, want global", app.Profile)
	}
	if app.BaseURL != "https://global.example" {
		t.Fatalf("BaseURL = %q, want global profile URL", app.BaseURL)
	}
	if app.ConfigScope != config.ScopeGlobal {
		t.Fatalf("ConfigScope = %q, want global", app.ConfigScope)
	}
	if app.ConfigDir != globalDir {
		t.Fatalf("ConfigDir = %q, want %q", app.ConfigDir, globalDir)
	}
}

func TestBuildAppUsesDefaultClientIDForHostedBaseURL(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvProfile, "")

	app, err := buildApp(&rootFlags{})
	if err != nil {
		t.Fatal(err)
	}

	if app.ClientID != DefaultClientID {
		t.Fatalf("ClientID = %q, want %q", app.ClientID, DefaultClientID)
	}
}

func TestBuildAppNormalizesHostedAccountURL(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvProfile, "")

	app, err := buildApp(&rootFlags{baseURL: "https://weeks.app/account/"})
	if err != nil {
		t.Fatal(err)
	}

	if app.BaseURL != config.DefaultBaseURL {
		t.Fatalf("BaseURL = %q, want %q", app.BaseURL, config.DefaultBaseURL)
	}
	if app.ClientID != DefaultClientID {
		t.Fatalf("ClientID = %q, want %q", app.ClientID, DefaultClientID)
	}
}

func TestBuildAppLeavesClientIDEmptyForCustomBaseURL(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "")
	t.Setenv(config.EnvProfile, "")

	app, err := buildApp(&rootFlags{baseURL: "http://localhost:3000"})
	if err != nil {
		t.Fatal(err)
	}

	if app.ClientID != "" {
		t.Fatalf("ClientID = %q, want empty for a custom base URL", app.ClientID)
	}
}

func TestBuildAppEnvClientIDOverridesProfileClientID(t *testing.T) {
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvBaseURL, "")
	t.Setenv(config.EnvClientID, "env-client")
	t.Setenv(config.EnvProfile, "")

	store := bcprofile.NewStore(config.ProfilesPath())
	clientID, err := json.Marshal("local-client")
	if err != nil {
		t.Fatal(err)
	}
	createErr := store.Create(&bcprofile.Profile{
		Name:    "local",
		BaseURL: "http://localhost:3000",
		Extra:   map[string]json.RawMessage{"client_id": clientID},
	})
	if createErr != nil {
		t.Fatal(createErr)
	}

	app, err := buildApp(&rootFlags{})
	if err != nil {
		t.Fatal(err)
	}

	if app.ClientID != "env-client" {
		t.Fatalf("ClientID = %q, want env-client", app.ClientID)
	}
}

func TestPromptSetupStoreDefaultsToLocal(t *testing.T) {
	var errOut bytes.Buffer

	got, err := promptSetupStore(strings.NewReader("\n"), &errOut)
	if err != nil {
		t.Fatal(err)
	}

	if got != config.ScopeLocal {
		t.Fatalf("store = %q, want local", got)
	}
	if !strings.Contains(errOut.String(), "Local folder ./.weeks") {
		t.Fatalf("prompt = %q", errOut.String())
	}
}

func TestPromptSetupStoreCanChooseGlobal(t *testing.T) {
	got, err := promptSetupStore(strings.NewReader("2\n"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	if got != config.ScopeGlobal {
		t.Fatalf("store = %q, want global", got)
	}
}
