// Package config resolves where weeks-cli keeps its state and which weeks
// installation a command talks to.
package config

import (
	"os"
	"path/filepath"
)

// DefaultBaseURL is the hosted weeks installation. A profile, WEEKS_BASE_URL,
// or --base-url overrides it — which is how a collection points the CLI at the
// Rails server running on its own WEEKS_APP_PORT.
const DefaultBaseURL = "https://weeks.app"

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

// Dir returns the config directory, creating nothing. Order: WEEKS_CONFIG_DIR,
// then $XDG_CONFIG_HOME/weeks, then ~/.config/weeks.
func Dir() string {
	if dir := os.Getenv(EnvConfigDir); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "weeks")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No home directory is recoverable: fall back to the working
		// directory rather than failing a command that may not need config
		// at all (`weeks --help`, `weeks commands --json`).
		return ".weeks"
	}
	return filepath.Join(home, ".config", "weeks")
}

// Path returns a path inside the config directory.
func Path(name string) string { return filepath.Join(Dir(), name) }

// ProfilesPath is where named profiles live.
func ProfilesPath() string { return Path("config.json") }

// BaseURLFromEnv returns the base URL override from the environment, if any.
func BaseURLFromEnv() string { return os.Getenv(EnvBaseURL) }
