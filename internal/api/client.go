// Package api is the small authenticated JSON client used by read commands.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/weeks-app/weeks-cli/internal/auth"
	"github.com/weeks-app/weeks-cli/internal/output"
)

// Client calls a weeks API using one stored OAuth credential.
type Client struct {
	BaseURL    string
	Profile    string
	Creds      *auth.Store
	HTTPClient *http.Client
}

// GetJSON GETs path and decodes the JSON response into the value the API
// returned: a map for one resource, a slice for a collection.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values) (any, error) {
	creds, err := c.Creds.Load(c.Profile, c.BaseURL)
	if err != nil {
		return nil, output.ErrAuth("not signed in; run `weeks auth login`")
	}
	if creds.Expired() {
		return nil, output.ErrAuth("stored token is expired; run `weeks auth login`")
	}

	endpoint, err := c.endpoint(path, query)
	if err != nil {
		return nil, output.ErrUsage(err.Error())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, output.ErrUsage(err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, output.ErrNetwork(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, output.ErrNetwork(err)
	}
	if resp.StatusCode >= 400 {
		return nil, responseError(resp.StatusCode, body)
	}

	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, output.ErrAPI(resp.StatusCode, fmt.Sprintf("API response was not JSON: %v", err))
	}
	return data, nil
}

func (c *Client) endpoint(path string, query url.Values) (string, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(rel.Path, "/")
	values := rel.Query()
	for key, set := range query {
		for _, value := range set {
			values.Add(key, value)
		}
	}
	base.RawQuery = values.Encode()
	return base.String(), nil
}

func responseError(status int, body []byte) error {
	message := errorMessage(body)
	switch status {
	case http.StatusUnauthorized:
		return output.ErrAuth("the stored token was rejected; run `weeks auth login`")
	case http.StatusForbidden:
		return &output.Error{Code: output.CodeForbidden, Message: fallback(message, "the token is not permitted to read this resource")}
	case http.StatusNotFound:
		return &output.Error{Code: output.CodeNotFound, Message: fallback(message, "resource not found")}
	default:
		return output.ErrAPI(status, fallback(message, "no error body"))
	}
}

func errorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body))
	}
	if msg, ok := payload["error"].(string); ok {
		return msg
	}
	if errors, ok := payload["errors"]; ok {
		encoded, _ := json.Marshal(errors)
		return string(encoded)
	}
	return strings.TrimSpace(string(body))
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
