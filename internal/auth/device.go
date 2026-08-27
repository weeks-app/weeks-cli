package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Device-flow endpoints, as mounted by doorkeeper-device_authorization_grant.
const (
	deviceAuthorizePath = "/oauth/authorize/device"
	tokenPath           = "/oauth/token"
	authorizePath       = "/oauth/authorize"
	revokePath          = "/oauth/revoke"

	// DeviceGrantType is the RFC 8628 grant type identifier.
	DeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

// DeviceAuthorization is the provider's answer to a device authorization
// request: the codes and where to take the user.
type DeviceAuthorization struct {
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`

	// VerificationURI is where the user types the user code.
	VerificationURI string `json:"verification_uri"`

	// VerificationURIComplete embeds the user code in the URL so the user only
	// has to follow a link. Optional in RFC 8628; Doorkeeper emits it.
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`

	ExpiresIn int `json:"expires_in"`
	Interval  int `json:"interval,omitempty"`
}

// PollInterval is how long to wait between token polls, honoring the
// provider's requested interval and RFC 8628's five-second default.
func (d *DeviceAuthorization) PollInterval() time.Duration {
	if d.Interval > 0 {
		return time.Duration(d.Interval) * time.Second
	}
	return 5 * time.Second
}

// Deadline is when the device code stops being usable.
func (d *DeviceAuthorization) Deadline(from time.Time) time.Time {
	if d.ExpiresIn > 0 {
		return from.Add(time.Duration(d.ExpiresIn) * time.Second)
	}
	return from.Add(10 * time.Minute)
}

// Client talks OAuth to one weeks installation.
type Client struct {
	BaseURL  string
	ClientID string
	HTTP     *http.Client

	// Sleep is how the device poll loop waits between attempts. It is a field
	// because the loop's timing is part of what needs testing — the RFC 8628
	// back-off, the deadline — and a test that waited real seconds for each
	// step would take a minute to assert something that takes microseconds to
	// compute. Left nil, it waits for real.
	Sleep func(ctx context.Context, d time.Duration) error
}

// wait blocks for d, or until ctx ends.
func (c *Client) wait(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// NewClient builds a client for a base URL and OAuth client id.
func NewClient(baseURL, clientID string) *Client {
	return &Client{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		ClientID: clientID,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) endpoint(path string) string { return c.BaseURL + path }

// tokenError is the RFC 6749 error body, which the device flow overloads to
// carry its polling states (authorization_pending, slow_down).
type tokenError struct {
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	ErrorURI         string `json:"error_uri,omitempty"`
}

func (e *tokenError) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("%s: %s", e.ErrorCode, e.ErrorDescription)
	}
	return e.ErrorCode
}

// Polling states from RFC 8628 §3.5. These are not failures — they are the
// protocol telling the client to keep waiting.
const (
	errAuthorizationPending = "authorization_pending"
	errSlowDown             = "slow_down"
	errAccessDenied         = "access_denied"
	errExpiredToken         = "expired_token"
)

// ErrDeviceDenied means the user rejected the authorization request.
var ErrDeviceDenied = errors.New("authorization was denied")

// ErrDeviceExpired means the user code expired before it was entered.
var ErrDeviceExpired = errors.New("the device code expired before it was approved")

// RequestDeviceCode starts a device authorization.
func (c *Client) RequestDeviceCode(ctx context.Context, scope string) (*DeviceAuthorization, error) {
	form := url.Values{"client_id": {c.ClientID}}
	if scope != "" {
		form.Set("scope", scope)
	}

	body, err := c.postForm(ctx, c.endpoint(deviceAuthorizePath), form)
	if err != nil {
		return nil, err
	}

	var auth DeviceAuthorization
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, fmt.Errorf("device authorization response was not JSON: %w", err)
	}
	if auth.DeviceCode == "" || auth.UserCode == "" {
		return nil, fmt.Errorf("device authorization response was missing codes")
	}
	if auth.VerificationURI == "" {
		// Doorkeeper always sends one; a proxy that strips it would leave the
		// user with a code and nowhere to enter it, which is worse than a
		// clear failure here.
		return nil, fmt.Errorf("device authorization response was missing verification_uri")
	}
	return &auth, nil
}

// PollDeviceToken polls until the user approves, denies, or the code expires.
// onSlowDown, when non-nil, is called with the new interval each time the
// provider asks the client to back off.
func (c *Client) PollDeviceToken(ctx context.Context, auth *DeviceAuthorization, onSlowDown func(time.Duration)) (*Credentials, error) {
	interval := auth.PollInterval()
	deadline := auth.Deadline(time.Now())

	for {
		if err := c.wait(ctx, interval); err != nil {
			return nil, err
		}

		if time.Now().After(deadline) {
			return nil, ErrDeviceExpired
		}

		creds, err := c.exchange(ctx, url.Values{
			"grant_type":  {DeviceGrantType},
			"device_code": {auth.DeviceCode},
			"client_id":   {c.ClientID},
		})
		if err == nil {
			return creds, nil
		}

		var terr *tokenError
		if !errors.As(err, &terr) {
			return nil, err
		}

		switch terr.ErrorCode {
		case errAuthorizationPending:
			// The user has not finished yet. Keep the same interval.
		case errSlowDown:
			// RFC 8628 §3.5: add five seconds and carry on.
			interval += 5 * time.Second
			if onSlowDown != nil {
				onSlowDown(interval)
			}
		case errAccessDenied:
			return nil, ErrDeviceDenied
		case errExpiredToken:
			return nil, ErrDeviceExpired
		default:
			return nil, err
		}
	}
}

// ExchangeCode trades an authorization code for tokens, completing the PKCE
// browser flow.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, verifier string) (*Credentials, error) {
	return c.exchange(ctx, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {c.ClientID},
		"code_verifier": {verifier},
	})
}

// Refresh trades a refresh token for a fresh access token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*Credentials, error) {
	return c.exchange(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.ClientID},
	})
}

// Revoke asks the provider to invalidate a token. A provider that does not
// mount the revocation endpoint is not an error the user can act on, so the
// caller decides whether to care.
func (c *Client) Revoke(ctx context.Context, token string) error {
	_, err := c.postForm(ctx, c.endpoint(revokePath), url.Values{
		"token":     {token},
		"client_id": {c.ClientID},
	})
	return err
}

// AuthorizeURL builds the browser-flow authorization URL.
func (c *Client) AuthorizeURL(redirectURI, state, challenge, scope string) string {
	q := url.Values{
		"client_id":             {c.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	if scope != "" {
		q.Set("scope", scope)
	}
	return c.endpoint(authorizePath) + "?" + q.Encode()
}

// tokenResponse is the RFC 6749 success body.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
}

func (c *Client) exchange(ctx context.Context, form url.Values) (*Credentials, error) {
	body, err := c.postForm(ctx, c.endpoint(tokenPath), form)
	if err != nil {
		return nil, err
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("token response was not JSON: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token response contained no access token")
	}

	creds := &Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		Scope:        tr.Scope,
		BaseURL:      c.BaseURL,
		ClientID:     c.ClientID,
		CreatedAt:    time.Now().UTC(),
	}
	if tr.ExpiresIn > 0 {
		creds.ExpiresAt = creds.CreatedAt.Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return creds, nil
}

// postForm posts a form and returns the body, turning an OAuth error body into
// a *tokenError so callers can branch on the code rather than on a string.
func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 1 MiB is far past any token response and bounds a misconfigured host
	// that answers this endpoint with a web page.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", endpoint, err)
	}

	if resp.StatusCode >= 400 {
		var terr tokenError
		if json.Unmarshal(body, &terr) == nil && terr.ErrorCode != "" {
			return nil, &terr
		}
		return nil, fmt.Errorf("%s returned %s", endpoint, resp.Status)
	}
	return body, nil
}
