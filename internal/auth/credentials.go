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
	inner *credstore.Store
}

// NewStore opens the credential store for this machine.
//
// Opening it probes the system keyring, and on a locked keychain that probe
// can block. The toolkit grew a bounded probe after v0.2.1, its newest tag;
// until that ships, WEEKS_NO_KEYRING=1 is the escape hatch for a headless or
// locked machine, and `weeks doctor` reports which store is actually in use.
func NewStore() *Store {
	return &Store{
		inner: credstore.NewStore(credstore.StoreOptions{
			ServiceName:   ServiceName,
			DisableEnvVar: config.EnvNoKeyring,
			FallbackDir:   config.Dir(),
		}),
	}
}

// UsingKeyring reports whether credentials go to the system keyring.
func (s *Store) UsingKeyring() bool { return s.inner.UsingKeyring() }

// FallbackWarning describes why the keyring was not used, or "" if it was.
func (s *Store) FallbackWarning() string { return s.inner.FallbackWarning() }

// key names the credential slot for a profile. Without a profile the base URL
// is the slot, so switching installations without naming a profile still
// keeps the two token sets apart.
func key(profileName, baseURL string) string {
	return profile.CredentialKey(profileName, baseURL)
}

// Load reads the credentials for a profile.
func (s *Store) Load(profileName, baseURL string) (*Credentials, error) {
	data, err := s.inner.Load(key(profileName, baseURL))
	if err != nil {
		return nil, err
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("stored credentials are unreadable: %w", err)
	}
	return &creds, nil
}

// Save writes the credentials for a profile.
func (s *Store) Save(profileName string, creds *Credentials) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	return s.inner.Save(key(profileName, creds.BaseURL), data)
}

// Delete removes the credentials for a profile.
func (s *Store) Delete(profileName, baseURL string) error {
	return s.inner.Delete(key(profileName, baseURL))
}
