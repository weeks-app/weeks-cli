package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/config"
	"github.com/weeks-app/weeks-cli/internal/output"
)

func TestGetJSONUsesStoredBearerTokenAndQuery(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	var gotPath, gotQuery, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"space_abc","name":"Studio"}]`))
	}))
	defer server.Close()

	store := auth.NewStore()
	if err := store.Save("", &auth.Credentials{AccessToken: "tok", BaseURL: server.URL}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := (&Client{BaseURL: server.URL, Creds: store}).GetJSON(context.Background(), "/api/v1/teams/team_abc/spaces?include=counts", nil)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if gotPath != "/api/v1/teams/team_abc/spaces" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "include=counts" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if len(data.([]any)) != 1 {
		t.Fatalf("data = %#v", data)
	}
}

func TestGetJSONRequiresCredentials(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	_, err := (&Client{BaseURL: "https://weeks.test", Creds: auth.NewStore()}).GetJSON(context.Background(), "/api/v1/teams", nil)
	if output.AsError(err).Code != output.CodeAuth {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
}

func TestGetJSONMapsHTTPStatus(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	defer server.Close()

	store := auth.NewStore()
	if err := store.Save("", &auth.Credentials{AccessToken: "tok", BaseURL: server.URL}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := (&Client{BaseURL: server.URL, Creds: store}).GetJSON(context.Background(), "/api/v1/spaces/space_abc", nil)
	if output.AsError(err).Code != output.CodeForbidden {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
}

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "weeks-api-test")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	os.Exit(m.Run())
}
