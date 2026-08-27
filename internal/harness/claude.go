// Package harness detects the agent harnesses installed on this machine, so
// `weeks setup` and `weeks doctor` can talk about them concretely.
package harness

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// PluginName is how weeks appears in a Claude Code plugin listing.
const PluginName = "weeks"

// StatusCheck is one harness observation, shaped like a doctor check so it can
// be reported as one without translation.
type StatusCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// ClaudeDir returns the Claude Code config directory.
func ClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// DetectClaude reports whether Claude Code is configured on this machine.
func DetectClaude() bool {
	dir := ClaudeDir()
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// FindClaudeBinary returns the path to the claude binary, or "" if there is
// none on PATH or in the usual install location.
func FindClaudeBinary() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(home, ".local", "bin", "claude")
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

// SkillDir returns where a personal Claude Code skill for weeks belongs.
func SkillDir() string {
	dir := ClaudeDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "skills", PluginName)
}

// CheckClaudePlugin reports whether the weeks skill is installed for Claude Code.
//
// The installed-plugins file is written by another tool and its shape is not a
// contract, so this reads it defensively: try the documented shapes, and fall
// back to searching the raw bytes. A false negative here costs an unnecessary
// `weeks setup claude`; a crash costs the whole doctor run.
func CheckClaudePlugin() *StatusCheck {
	const name = "Claude Code skill"

	if !DetectClaude() {
		return &StatusCheck{
			Name:    name,
			Status:  "skip",
			Message: "Claude Code is not installed on this machine",
		}
	}

	if dir := SkillDir(); dir != "" {
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
			return &StatusCheck{Name: name, Status: "pass", Message: "installed at " + dir}
		}
	}

	if pluginInstalled() {
		return &StatusCheck{Name: name, Status: "pass", Message: "installed as a Claude Code plugin"}
	}

	return &StatusCheck{
		Name:    name,
		Status:  "warn",
		Message: "the weeks skill is not installed for Claude Code",
		Hint:    "Run: weeks setup claude",
	}
}

func pluginInstalled() bool {
	dir := ClaudeDir()
	if dir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "plugins", "installed_plugins.json")) //nolint:gosec // G304: path is derived from the user's own home directory
	if err != nil {
		return false
	}

	var asList []map[string]any
	if json.Unmarshal(data, &asList) == nil {
		for _, entry := range asList {
			for _, field := range []string{"name", "package", "id"} {
				if s, ok := entry[field].(string); ok && matchesPlugin(s) {
					return true
				}
			}
		}
	}

	var asMap map[string]any
	if json.Unmarshal(data, &asMap) == nil {
		for key := range asMap {
			if matchesPlugin(key) {
				return true
			}
		}
	}

	// Last resort: the file mentions us somewhere. Loose, but the cost of
	// being wrong is only a redundant setup suggestion.
	return bytes.Contains(data, []byte(PluginName))
}

func matchesPlugin(s string) bool {
	return s == PluginName || s == PluginName+"@"+PluginName
}
