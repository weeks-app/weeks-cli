// Package auth authenticates weeks-cli against a weeks installation's
// Doorkeeper provider and keeps the resulting credentials.
//
// Two grants are supported, both against the same provider:
//
//   - the device authorization grant (RFC 8628), which is the default because
//     it is the only one that works where an agent usually runs — an SSH
//     session, a container, a pane with no browser of its own;
//   - the authorization code grant with PKCE (RFC 7636), used by
//     `weeks auth login --browser` when there is a desktop to hand the
//     browser to.
//
// Credentials are stored per profile, so `weeks --profile acme` and
// `weeks --profile staging` never see each other's tokens. That per-profile
// isolation is the CLI's tenant boundary.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/basecamp/cli/credstore"
	"github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/config"
)

// ServiceName is the keyring service weeks-cli stores credentials under.
const ServiceName = "weeks"

// Credentials is one profile's stored token set.
type Credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	BaseURL      string    `json:"base_url"`
	ClientID     string    `json:"client_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Expired reports whether the access token is past its expiry. A credential
// with no recorded expiry never reports expired: Doorkeeper can be configured
// to issue non-expiring tokens, and guessing an expiry would log the user out
// of a perfectly good session.
func (c *Credentials) Expired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// ExpiringWithin reports whether the token expires inside d. Used to refresh
// ahead of a request rather than after it has already failed.
func (c *Credentials) ExpiringWithin(d time.Duration) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(d).After(c.ExpiresAt)
}

// Store persists credentials, preferring the system keyring and falling back
// to a 0600 file when there is no keyring to reach.
type Store struct {
	inner     *credstore.Store
	fileDir   string
	forceFile bool
}

// NewStore opens the credential store for this machine.
//
// Opening it probes the system keyring, and on a locked keychain that probe
// can block. The toolkit grew a bounded probe after v0.2.1, its newest tag;
// until that ships, WEEKS_NO_KEYRING=1 is the escape hatch for a headless or
// locked machine, and `weeks doctor` reports which store is actually in use.
func NewStore(configDir string) *Store {
	if configDir == "" {
		configDir = config.Dir()
	}
	return &Store{
		fileDir: configDir,
		inner: credstore.NewStore(credstore.StoreOptions{
			ServiceName:   ServiceName,
			DisableEnvVar: config.EnvNoKeyring,
			FallbackDir:   configDir,
		}),
	}
}

// NewFileStore opens a file-only credential store rooted at configDir.
func NewFileStore(configDir string) *Store {
	if configDir == "" {
		configDir = config.Dir()
	}
	return &Store{fileDir: configDir, forceFile: true}
}

// UsingKeyring reports whether credentials go to the system keyring.
func (s *Store) UsingKeyring() bool {
	return !s.forceFile && s.inner.UsingKeyring()
}

// FallbackWarning describes why the keyring was not used, or "" if it was.
func (s *Store) FallbackWarning() string {
	if s.forceFile {
		return ""
	}
	return s.inner.FallbackWarning()
}

// FilePath returns the credentials file path when credentials are file-backed.
func (s *Store) FilePath() string {
	if s.fileDir != "" {
		return filepath.Join(s.fileDir, "credentials.json")
	}
	return ""
}

// key names the credential slot for a profile. Without a profile the base URL
// is the slot, so switching installations without naming a profile still
// keeps the two token sets apart.
func key(profileName, baseURL string) string {
	return profile.CredentialKey(profileName, baseURL)
}

// Load reads the credentials for a profile.
func (s *Store) Load(profileName, baseURL string) (*Credentials, error) {
	data, err := s.load(key(profileName, baseURL))
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("stored credentials are unreadable: %w", err)
	}
	return &creds, nil
}

// IsNotFound reports whether err means there is no stored credential.
func IsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "credentials not found")
}

// Save writes the credentials for a profile.
func (s *Store) Save(profileName string, creds *Credentials) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	return s.save(key(profileName, creds.BaseURL), data)
}

// Delete removes the credentials for a profile.
func (s *Store) Delete(profileName, baseURL string) error {
	return s.delete(key(profileName, baseURL))
}

func (s *Store) load(name string) ([]byte, error) {
	if !s.forceFile {
		return s.inner.Load(name)
	}
	var data []byte
	err := s.withFileLock(func() error {
		all, err := s.loadAllFromFile()
		if err != nil {
			return err
		}
		found, ok := all[name]
		if !ok {
			return fmt.Errorf("credentials not found for %s", name)
		}
		data = append([]byte(nil), found...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Store) save(name string, data []byte) error {
	if !s.forceFile {
		return s.inner.Save(name, data)
	}
	return s.withFileLock(func() error {
		all, err := s.loadAllFromFile()
		if err != nil {
			return err
		}
		all[name] = data
		return s.saveAllToFile(all)
	})
}

func (s *Store) delete(name string) error {
	if !s.forceFile {
		return s.inner.Delete(name)
	}
	return s.withFileLock(func() error {
		all, err := s.loadAllFromFile()
		if err != nil {
			return err
		}
		delete(all, name)
		return s.saveAllToFile(all)
	})
}

func (s *Store) withFileLock(fn func() error) error {
	if err := os.MkdirAll(s.fileDir, 0700); err != nil {
		return err
	}

	lockPath := filepath.Join(s.fileDir, "credentials.json.lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := os.Mkdir(lockPath, 0700)
		if err == nil {
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for credentials file lock")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *Store) loadAllFromFile() (map[string][]byte, error) {
	data, err := os.ReadFile(s.FilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]byte), nil
		}
		return nil, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	all := make(map[string][]byte, len(raw))
	for k, v := range raw {
		all[k] = []byte(v)
	}
	return all, nil
}

func (s *Store) saveAllToFile(all map[string][]byte) error {
	if err := os.MkdirAll(s.fileDir, 0700); err != nil {
		return err
	}

	raw := make(map[string]json.RawMessage, len(all))
	for k, v := range all {
		raw[k] = json.RawMessage(v)
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(s.fileDir, "credentials-*.json.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	destPath := s.FilePath()
	if err := os.Rename(tmpPath, destPath); err != nil {
		if runtime.GOOS == "windows" {
			return replaceFileWindows(tmpPath, destPath, s.fileDir)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func replaceFileWindows(tmpPath, destPath, dir string) error {
	backupFile, err := os.CreateTemp(dir, "credentials-*.json.bak")
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		_ = os.Remove(backupPath)
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	hadDest := false
	if err := os.Rename(destPath, backupPath); err != nil {
		if !os.IsNotExist(err) {
			_ = os.Remove(tmpPath)
			return err
		}
	} else {
		hadDest = true
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		if hadDest {
			_ = os.Rename(backupPath, destPath)
		}
		_ = os.Remove(tmpPath)
		return err
	}

	if hadDest {
		_ = os.Remove(backupPath)
	}
	return nil
}
