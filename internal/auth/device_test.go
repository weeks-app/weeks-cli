package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/weeks-app/weeks-cli/internal/auth"
)

// deviceServer stands in for Doorkeeper with the device grant mounted. Each
// token poll pops the next scripted response, so a test can describe a whole
// conversation rather than one exchange.
type deviceServer struct {
	t         *testing.T
	responses []string
	polls     int
}

func (d *deviceServer) start() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/oauth/authorize_device", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			d.t.Fatalf("device request was not a form: %v", err)
		}
		if r.Form.Get("client_id") == "" {
			d.t.Error("device request carried no client_id")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"device_code": "dev-code",
			"user_code": "WXYZ-1234",
			"verification_uri": "https://weeks.test/device",
			"verification_uri_complete": "https://weeks.test/device?user_code=WXYZ-1234",
			"expires_in": 300,
			"interval": 0
		}`))
	})

	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		body := d.responses[min(d.polls, len(d.responses)-1)]
		d.polls++
		w.Header().Set("Content-Type", "application/json")
		if body[0] == '!' {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(body[1:]))
			return
		}
		_, _ = w.Write([]byte(body))
	})

	server := httptest.NewServer(mux)
	d.t.Cleanup(server.Close)
	return server
}

// fastClient is a client whose poll loop does not actually wait. The waits it
// skips are asserted on separately, through the onSlowDown callback.
func fastClient(url string) *auth.Client {
	c := auth.NewClient(url, "client-uid")
	c.Sleep = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	return c
}

func TestRequestDeviceCode(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{`{}`}}).start()

	da, err := auth.NewClient(srv.URL, "client-uid").RequestDeviceCode(context.Background(), "")
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}

	if da.UserCode != "WXYZ-1234" {
		t.Errorf("user code = %q", da.UserCode)
	}
	if da.VerificationURIComplete == "" {
		t.Error("the complete URI is what spares the user retyping the code")
	}
	// RFC 8628 says five seconds when the server does not say otherwise.
	if got := da.PollInterval(); got != 5*time.Second {
		t.Errorf("poll interval = %v, want the RFC default of 5s", got)
	}
}

func TestPollDeviceTokenWaitsThroughAuthorizationPending(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{
		`!{"error":"authorization_pending"}`,
		`!{"error":"authorization_pending"}`,
		`{"access_token":"tok","refresh_token":"ref","token_type":"Bearer","scope":"admin","expires_in":7200}`,
	}}).start()

	creds, err := pollFast(t, fastClient(srv.URL), &auth.DeviceAuthorization{DeviceCode: "dev-code", ExpiresIn: 300})
	if err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}

	if creds.AccessToken != "tok" {
		t.Errorf("access token = %q", creds.AccessToken)
	}
	if creds.RefreshToken != "ref" {
		t.Errorf("refresh token = %q", creds.RefreshToken)
	}
	if creds.ExpiresAt.IsZero() {
		t.Error("expires_in was not turned into an expiry")
	}
	if creds.BaseURL != srv.URL {
		t.Errorf("base URL = %q, want the installation it was issued by", creds.BaseURL)
	}
}

func TestPollDeviceTokenReportsDenial(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{`!{"error":"access_denied"}`}}).start()

	_, err := pollFast(t, fastClient(srv.URL), &auth.DeviceAuthorization{DeviceCode: "d", ExpiresIn: 300})
	if !errors.Is(err, auth.ErrDeviceDenied) {
		t.Errorf("err = %v, want ErrDeviceDenied", err)
	}
}

func TestPollDeviceTokenReportsExpiry(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{`!{"error":"expired_token"}`}}).start()

	_, err := pollFast(t, fastClient(srv.URL), &auth.DeviceAuthorization{DeviceCode: "d", ExpiresIn: 300})
	if !errors.Is(err, auth.ErrDeviceExpired) {
		t.Errorf("err = %v, want ErrDeviceExpired", err)
	}
}

func TestPollDeviceTokenBacksOffOnSlowDown(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{
		`!{"error":"slow_down"}`,
		`!{"error":"slow_down"}`,
		`{"access_token":"tok"}`,
	}}).start()

	var waited []time.Duration
	client := auth.NewClient(srv.URL, "client-uid")
	client.Sleep = func(_ context.Context, d time.Duration) error {
		waited = append(waited, d)
		return nil
	}

	if _, err := client.PollDeviceToken(context.Background(), &auth.DeviceAuthorization{DeviceCode: "d", ExpiresIn: 300}, nil); err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}

	// RFC 8628 §3.5: each slow_down adds five seconds to the interval.
	want := []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second}
	if len(waited) != len(want) {
		t.Fatalf("waited %v, want %v", waited, want)
	}
	for i := range want {
		if waited[i] != want[i] {
			t.Errorf("wait %d = %v, want %v", i, waited[i], want[i])
		}
	}
}

func TestPollDeviceTokenStopsWhenTheCodeHasExpired(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{`!{"error":"authorization_pending"}`}}).start()

	// A device code that lapses while the client is still waiting must be
	// reported as expired rather than polled against forever. The deadline is
	// real wall-clock time, so the wait here is too — one second, once.
	client := auth.NewClient(srv.URL, "client-uid")
	client.Sleep = func(_ context.Context, _ time.Duration) error {
		time.Sleep(1100 * time.Millisecond)
		return nil
	}

	_, err := client.PollDeviceToken(
		context.Background(),
		&auth.DeviceAuthorization{DeviceCode: "d", ExpiresIn: 1},
		nil,
	)
	if !errors.Is(err, auth.ErrDeviceExpired) {
		t.Errorf("err = %v, want ErrDeviceExpired", err)
	}
}

func TestPollDeviceTokenHonorsContextCancellation(t *testing.T) {
	srv := (&deviceServer{t: t, responses: []string{`!{"error":"authorization_pending"}`}}).start()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fastClient(srv.URL).PollDeviceToken(
		ctx, &auth.DeviceAuthorization{DeviceCode: "d", ExpiresIn: 300}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestAuthorizeURLCarriesPKCE(t *testing.T) {
	url := auth.NewClient("https://weeks.test", "uid").
		AuthorizeURL("http://127.0.0.1:5000/callback", "state-1", "challenge-1", "admin")

	for _, want := range []string{
		"code_challenge=challenge-1",
		"code_challenge_method=S256",
		"response_type=code",
		"state=state-1",
	} {
		if !contains(url, want) {
			t.Errorf("authorize URL is missing %q: %s", want, url)
		}
	}
}

func TestExchangeCodeSendsTheVerifier(t *testing.T) {
	var gotVerifier, gotGrant string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotVerifier = r.Form.Get("code_verifier")
		gotGrant = r.Form.Get("grant_type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok"}`))
	}))
	defer srv.Close()

	_, err := auth.NewClient(srv.URL, "uid").ExchangeCode(context.Background(), "code", "http://127.0.0.1:1/callback", "verifier-1")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if gotVerifier != "verifier-1" {
		t.Errorf("code_verifier = %q; without it PKCE proves nothing", gotVerifier)
	}
	if gotGrant != "authorization_code" {
		t.Errorf("grant_type = %q", gotGrant)
	}
}

func TestCredentialsWithoutExpiryNeverLookExpired(t *testing.T) {
	// Doorkeeper can issue non-expiring tokens. Guessing an expiry would log
	// a working session out.
	c := &auth.Credentials{AccessToken: "tok"}
	if c.Expired() {
		t.Error("a credential with no expiry reported itself expired")
	}
	if c.ExpiringWithin(365 * 24 * time.Hour) {
		t.Error("a credential with no expiry reported itself expiring")
	}
}

func TestCredentialsExpiry(t *testing.T) {
	past := &auth.Credentials{ExpiresAt: time.Now().Add(-time.Minute)}
	if !past.Expired() {
		t.Error("a past expiry did not report expired")
	}

	soon := &auth.Credentials{ExpiresAt: time.Now().Add(30 * time.Minute)}
	if soon.Expired() {
		t.Error("a future expiry reported expired")
	}
	if !soon.ExpiringWithin(time.Hour) {
		t.Error("a token expiring in 30 minutes is expiring within an hour")
	}
	if soon.ExpiringWithin(time.Minute) {
		t.Error("a token expiring in 30 minutes is not expiring within a minute")
	}
}

// pollFast runs a poll with a context that gives up rather than hanging a test
// run if the client's loop ever stops making progress.
func pollFast(t *testing.T, client *auth.Client, da *auth.DeviceAuthorization) (*auth.Credentials, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return client.PollDeviceToken(ctx, da, nil)
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
