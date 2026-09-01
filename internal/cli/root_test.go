package cli

import (
	"encoding/json"
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
