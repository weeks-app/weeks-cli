package auth

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"

	"github.com/basecamp/cli/oauthcallback"
	"github.com/basecamp/cli/pkce"
)

// BrowserLogin runs the authorization code grant with PKCE: bind a loopback
// listener, send the user to the provider, and trade the returned code for
// tokens using the verifier that never left this process.
//
// The listener is bound before the URL is built because the redirect URI has
// to name the port the provider will redirect to, and asking the OS for a free
// port is the only way to know it. announce is called with the URL once it is
// known, so the caller decides whether to print it, open it, or both.
func BrowserLogin(ctx context.Context, c *Client, scope string, announce func(url string)) (*Credentials, error) {
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("could not open a local callback port: %w", err)
	}
	// WaitForCallback closes the listener; closing it here as well would be a
	// double close, so ownership passes with the call below.

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	verifier := pkce.GenerateVerifier()
	challenge := pkce.GenerateChallenge(verifier)
	state := pkce.GenerateState()

	authURL := c.AuthorizeURL(redirectURI, state, challenge, scope)
	if announce != nil {
		announce(authURL)
	}

	code, err := oauthcallback.WaitForCallback(ctx, state, listener, "")
	if err != nil {
		return nil, err
	}
	return c.ExchangeCode(ctx, code, redirectURI, verifier)
}

// OpenBrowser tries to hand a URL to the desktop browser. Failing is normal —
// there is often no desktop — so the caller prints the URL either way and
// treats the error as advisory.
func OpenBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)

	return exec.Command(cmd, args...).Start() //nolint:gosec // G204: url is built by AuthorizeURL from a configured base URL
}
