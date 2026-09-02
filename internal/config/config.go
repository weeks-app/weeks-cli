// Package config resolves where weeks-cli keeps its state and which weeks
// installation a command talks to.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultBaseURL is the hosted weeks installation. A profile, WEEKS_BASE_URL,
// or --base-url overrides it — which is how a collection points the CLI at the
// Rails server running on its own WEEKS_APP_PORT.
const DefaultBaseURL = "https://weeks.app"

// DefaultHostedClientID is the public, device-only OAuth client id shipped for
// the hosted weeks installation.
const DefaultHostedClientID = "weeks-cli"

// IsHostedBaseURL reports whether baseURL points at the hosted weeks app. It
// accepts /account links too because NormalizeBaseURL folds them to the
// canonical installation root. Only that installation can rely on the CLI's
// built-in OAuth client id.
func IsHostedBaseURL(baseURL string) bool {
	return NormalizeBaseURL(baseURL) == DefaultBaseURL
}

// NormalizeBaseURL folds product-page links for the hosted app back to the
// installation root that owns the OAuth and API endpoints.
func NormalizeBaseURL(baseURL string) string {
	normalized := strings.TrimRight(baseURL, "/")
	if normalized == DefaultBaseURL+"/account" {
		return DefaultBaseURL
	}
	return normalized
}

// EnvBaseURL overrides the base URL for a single invocation.
const EnvBaseURL = "WEEKS_BASE_URL"

// EnvProfile selects a named profile without passing --profile.
const EnvProfile = "WEEKS_PROFILE"

// EnvConfigDir relocates the whole config directory. Tests and the e2e suite
// set it so they never touch a developer's real credentials.
const EnvConfigDir = "WEEKS_CONFIG_DIR"

// EnvNoKeyring forces file-backed credential storage.
const EnvNoKeyring = "WEEKS_NO_KEYRING"

// EnvClientID overrides the OAuth client the device flow authenticates as.
// A self-hosted or development weeks has its own Platform::Application, so the
// built-in default cannot be right for every installation.
const EnvClientID = "WEEKS_CLIENT_ID"

const (
	ScopeLocal  = "local"
	ScopeGlobal = "global"
	ScopeEnv    = "env"
)

// Dir returns the default config directory, creating nothing. Order:
// WEEKS_CONFIG_DIR, then ./.weeks. Use ResolveDir(true) for the global path.
func Dir() string {
	dir, _ := ResolveDir(false)
	return dir
}

// ResolveDir returns the config directory and scope for one invocation.
func ResolveDir(useGlobal bool) (string, string) {
	if dir := os.Getenv(EnvConfigDir); dir != "" {
		return dir, ScopeEnv
	}
	if useGlobal {
		return GlobalDir(), ScopeGlobal
	}
	return LocalDir(), ScopeLocal
}

// LocalDir returns the current-folder config directory. This is the default so
// agents working in different folders do not share credentials by accident.
func LocalDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".weeks"
	}
	return filepath.Join(cwd, ".weeks")
}

// GlobalDir returns the user-wide config directory.
func GlobalDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "weeks")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No home directory is recoverable: fall back to the working
		// directory rather than failing a command that may not need config
		// at all (`weeks --help`, `weeks commands --json`).
		return ".weeks-global"
	}
	return filepath.Join(home, ".config", "weeks")
}

// Path returns a path inside the config directory.
func Path(name string) string { return filepath.Join(Dir(), name) }

// ProfilesPath is where named profiles live.
func ProfilesPath() string { return Path("config.json") }

// ProfilesPathIn is where named profiles live inside dir.
func ProfilesPathIn(dir string) string { return filepath.Join(dir, "config.json") }

// BaseURLFromEnv returns the base URL override from the environment, if any.
func BaseURLFromEnv() string { return os.Getenv(EnvBaseURL) }
