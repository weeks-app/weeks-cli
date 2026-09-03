package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	store := auth.NewFileStore(config.Dir())
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

	_, err := (&Client{BaseURL: "https://weeks.test", Creds: auth.NewFileStore(config.Dir())}).GetJSON(context.Background(), "/api/v1/teams", nil)
	if output.AsError(err).Code != output.CodeAuth {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
}

func TestGetJSONRefreshesExpiredCredentials(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	var gotRefreshToken, gotClientID, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				http.Error(w, "bad form", http.StatusInternalServerError)
				return
			}
			gotRefreshToken = r.Form.Get("refresh_token")
			gotClientID = r.Form.Get("client_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","token_type":"Bearer","scope":"admin","expires_in":7200}`))
		case "/api/v1/teams":
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"team_abc","name":"Ops"}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := auth.NewFileStore(config.Dir())
	if err := store.Save("", &auth.Credentials{
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		BaseURL:      server.URL,
		ClientID:     "stored-client",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := (&Client{BaseURL: server.URL, ClientID: "runtime-client", Creds: store}).GetJSON(context.Background(), "/api/v1/teams", nil)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if gotRefreshToken != "old-refresh" {
		t.Fatalf("refresh_token = %q, want old-refresh", gotRefreshToken)
	}
	if gotClientID != "stored-client" {
		t.Fatalf("client_id = %q, want stored-client", gotClientID)
	}
	if gotAuth != "Bearer new-token" {
		t.Fatalf("Authorization = %q, want refreshed token", gotAuth)
	}
	if len(data.([]any)) != 1 {
		t.Fatalf("data = %#v", data)
	}

	creds, err := store.Load("", server.URL)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.AccessToken != "new-token" || creds.RefreshToken != "new-refresh" || creds.ClientID != "stored-client" {
		t.Fatalf("stored credentials = %#v", creds)
	}
}

func TestGetJSONFallsBackToRuntimeClientIDForLegacyCredential(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	var gotClientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				http.Error(w, "bad form", http.StatusInternalServerError)
				return
			}
			gotClientID = r.Form.Get("client_id")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh","token_type":"Bearer","scope":"admin","expires_in":7200}`))
		case "/api/v1/teams":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := auth.NewFileStore(config.Dir())
	if err := store.Save("", &auth.Credentials{
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		BaseURL:      server.URL,
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := (&Client{BaseURL: server.URL, ClientID: "runtime-client", Creds: store}).GetJSON(context.Background(), "/api/v1/teams", nil); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if gotClientID != "runtime-client" {
		t.Fatalf("client_id = %q, want runtime-client", gotClientID)
	}
}

func TestGetJSONPreservesRefreshTokenWhenProviderDoesNotRotate(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","token_type":"Bearer","expires_in":7200}`))
		case "/api/v1/teams":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := auth.NewFileStore(config.Dir())
	if err := store.Save("", &auth.Credentials{
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		BaseURL:      server.URL,
		ClientID:     "stored-client",
		Scope:        "admin",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := (&Client{BaseURL: server.URL, Creds: store}).GetJSON(context.Background(), "/api/v1/teams", nil); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	creds, err := store.Load("", server.URL)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if creds.RefreshToken != "old-refresh" {
		t.Fatalf("refresh token = %q, want preserved old-refresh", creds.RefreshToken)
	}
	if creds.Scope != "admin" {
		t.Fatalf("scope = %q, want preserved admin", creds.Scope)
	}
}

func TestGetJSONReportsCredentialStoreFailureAfterRefresh(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	store := auth.NewFileStore(config.Dir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-token","token_type":"Bearer","expires_in":7200}`))
			if err := os.Chmod(config.Dir(), 0500); err != nil {
				t.Errorf("Chmod: %v", err)
			}
		case "/api/v1/teams":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer func() { _ = os.Chmod(config.Dir(), 0700) }()

	if err := store.Save("", &auth.Credentials{
		AccessToken:  "old-token",
		RefreshToken: "old-refresh",
		BaseURL:      server.URL,
		ClientID:     "stored-client",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := (&Client{BaseURL: server.URL, Creds: store}).GetJSON(context.Background(), "/api/v1/teams", nil)
	structured := output.AsError(err)
	if structured.Code != output.CodeAuth {
		t.Fatalf("code = %q, err = %v", structured.Code, err)
	}
	if !strings.Contains(structured.Message, "credential store") {
		t.Fatalf("message = %q", structured.Message)
	}
	if strings.Contains(structured.Message, "auth login") || strings.Contains(structured.Hint, "auth login") {
		t.Fatalf("misleading login guidance: message=%q hint=%q", structured.Message, structured.Hint)
	}
}

func TestGetJSONRequiresLoginWhenExpiredCredentialsCannotRefresh(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	store := auth.NewFileStore(config.Dir())
	if err := store.Save("", &auth.Credentials{
		AccessToken: "old-token",
		BaseURL:     server.URL,
		ExpiresAt:   time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := (&Client{BaseURL: server.URL, Creds: store}).GetJSON(context.Background(), "/api/v1/teams", nil)
	if output.AsError(err).Code != output.CodeAuth {
		t.Fatalf("code = %q, err = %v", output.AsError(err).Code, err)
	}
	if calls != 0 {
		t.Fatalf("server calls = %d, want none", calls)
	}
}

func TestGetJSONUsesValidCredentialThatCannotRefreshYet(t *testing.T) {
	t.Setenv(config.EnvNoKeyring, "1")
	t.Setenv("HOME", t.TempDir())

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/teams" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	store := auth.NewFileStore(config.Dir())
	if err := store.Save("", &auth.Credentials{
		AccessToken: "old-token",
		BaseURL:     server.URL,
		ExpiresAt:   time.Now().Add(30 * time.Second),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := (&Client{BaseURL: server.URL, Creds: store}).GetJSON(context.Background(), "/api/v1/teams", nil); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if gotAuth != "Bearer old-token" {
		t.Fatalf("Authorization = %q, want existing token", gotAuth)
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

	store := auth.NewFileStore(config.Dir())
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
	_ = os.Setenv(config.EnvConfigDir, filepath.Join(tmp, "config"))
	code := m.Run()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
