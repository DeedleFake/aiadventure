package xai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Public OAuth client id used by Grok CLI / SuperGrok device-code flow.
const (
	DefaultClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	DefaultIssuer   = "https://auth.x.ai"
	DefaultAPIBase  = "https://api.x.ai/v1"
	DefaultScope    = "openid profile email offline_access grok-cli:access api:access"
	deviceCodeGrant = "urn:ietf:params:oauth:grant-type:device_code"
	refreshSkew     = time.Hour
)

// TokenSet is a stored OAuth credential bundle.
type TokenSet struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	// TokenEndpoint is the OIDC token endpoint used for refresh.
	TokenEndpoint string `json:"token_endpoint,omitempty"`
	// APIBase is the inference base URL.
	APIBase string `json:"api_base,omitempty"`
	// UpdatedAt is when tokens were last written.
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// Valid reports whether an access token is present and not near expiry.
func (t TokenSet) Valid(now time.Time) bool {
	if t.AccessToken == "" {
		return false
	}
	if t.ExpiresAt.IsZero() {
		return true
	}
	return now.Before(t.ExpiresAt.Add(-refreshSkew))
}

// NeedsRefresh reports whether refresh should run before use.
func (t TokenSet) NeedsRefresh(now time.Time) bool {
	if t.AccessToken == "" {
		return t.RefreshToken != ""
	}
	if t.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(t.ExpiresAt.Add(-refreshSkew))
}

// TokenStore persists TokenSet as JSON.
type TokenStore struct {
	Path string
}

// Load reads tokens from disk. Missing file returns empty set, nil error.
func (s TokenStore) Load() (TokenSet, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return TokenSet{}, nil
		}
		return TokenSet{}, fmt.Errorf("read auth: %w", err)
	}
	var t TokenSet
	if err := json.Unmarshal(data, &t); err != nil {
		return TokenSet{}, fmt.Errorf("parse auth: %w", err)
	}
	return t, nil
}

// Save writes tokens with restrictive permissions.
func (s TokenStore) Save(t TokenSet) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("mkdir auth: %w", err)
	}
	t.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth: %w", err)
	}
	data = append(data, '\n')
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write auth tmp: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename auth: %w", err)
	}
	return nil
}

// Clear removes stored tokens.
func (s TokenStore) Clear() error {
	if err := os.Remove(s.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear auth: %w", err)
	}
	return nil
}

// OIDCDiscovery is the subset of OIDC metadata we need.
type OIDCDiscovery struct {
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

// DeviceCodeResponse is the device authorization response.
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// OAuthClient performs device-code login and token refresh.
type OAuthClient struct {
	HTTP     *http.Client
	ClientID string
	Issuer   string
	Scope    string
	APIBase  string
	Now      func() time.Time
}

func (c *OAuthClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *OAuthClient) clientID() string {
	if c.ClientID != "" {
		return c.ClientID
	}
	return DefaultClientID
}

func (c *OAuthClient) issuer() string {
	if c.Issuer != "" {
		return c.Issuer
	}
	return DefaultIssuer
}

func (c *OAuthClient) scope() string {
	if c.Scope != "" {
		return c.Scope
	}
	return DefaultScope
}

func (c *OAuthClient) apiBase() string {
	if c.APIBase != "" {
		return c.APIBase
	}
	return DefaultAPIBase
}

func (c *OAuthClient) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Discover fetches OIDC configuration from the issuer.
func (c *OAuthClient) Discover(ctx context.Context) (OIDCDiscovery, error) {
	u := strings.TrimRight(c.issuer(), "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return OIDCDiscovery{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return OIDCDiscovery{}, fmt.Errorf("oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return OIDCDiscovery{}, fmt.Errorf("oidc discovery: status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var d OIDCDiscovery
	if err := json.Unmarshal(body, &d); err != nil {
		return OIDCDiscovery{}, fmt.Errorf("oidc discovery parse: %w", err)
	}
	if d.TokenEndpoint == "" {
		return OIDCDiscovery{}, fmt.Errorf("oidc discovery: missing token_endpoint")
	}
	return d, nil
}

// RequestDeviceCode starts the device authorization grant.
func (c *OAuthClient) RequestDeviceCode(ctx context.Context, deviceURL string) (DeviceCodeResponse, error) {
	if deviceURL == "" {
		deviceURL = strings.TrimRight(c.issuer(), "/") + "/oauth2/device/code"
	}
	form := url.Values{
		"client_id": {c.clientID()},
		"scope":     {c.scope()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceURL, strings.NewReader(form.Encode()))
	if err != nil {
		return DeviceCodeResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("device code: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return DeviceCodeResponse{}, fmt.Errorf("device code: status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var dc DeviceCodeResponse
	if err := json.Unmarshal(body, &dc); err != nil {
		return DeviceCodeResponse{}, fmt.Errorf("device code parse: %w", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return DeviceCodeResponse{}, fmt.Errorf("device code: missing device_code or user_code")
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return dc, nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// PollDeviceToken waits until the user authorizes or the code expires.
func (c *OAuthClient) PollDeviceToken(ctx context.Context, tokenEndpoint string, dc DeviceCodeResponse) (TokenSet, error) {
	deadline := c.now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	if dc.ExpiresIn <= 0 {
		deadline = c.now().Add(15 * time.Minute)
	}
	interval := time.Duration(dc.Interval) * time.Second
	if dc.Interval <= 0 {
		interval = 5 * time.Second
	}

	for {
		if err := ctx.Err(); err != nil {
			return TokenSet{}, err
		}
		if c.now().After(deadline) {
			return TokenSet{}, fmt.Errorf("device authorization timed out")
		}

		form := url.Values{
			"grant_type":  {deviceCodeGrant},
			"client_id":   {c.clientID()},
			"device_code": {dc.DeviceCode},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return TokenSet{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return TokenSet{}, fmt.Errorf("poll token: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var tr tokenResponse
		_ = json.Unmarshal(body, &tr)

		if resp.StatusCode == http.StatusOK && tr.AccessToken != "" {
			return c.tokenSetFromResponse(tr, tokenEndpoint), nil
		}

		switch tr.Error {
		case "authorization_pending":
			if err := sleepCtx(ctx, interval); err != nil {
				return TokenSet{}, err
			}
			continue
		case "slow_down":
			interval += time.Second
			if interval > 30*time.Second {
				interval = 30 * time.Second
			}
			if err := sleepCtx(ctx, interval); err != nil {
				return TokenSet{}, err
			}
			continue
		default:
			msg := tr.Error
			if tr.ErrorDesc != "" {
				msg = tr.ErrorDesc
			}
			if msg == "" {
				msg = truncate(string(body), 200)
			}
			return TokenSet{}, fmt.Errorf("device token: %s", msg)
		}
	}
}

// Refresh exchanges a refresh token for a new access token.
func (c *OAuthClient) Refresh(ctx context.Context, tokens TokenSet) (TokenSet, error) {
	endpoint := tokens.TokenEndpoint
	if endpoint == "" {
		d, err := c.Discover(ctx)
		if err != nil {
			return TokenSet{}, err
		}
		endpoint = d.TokenEndpoint
	}
	if tokens.RefreshToken == "" {
		return TokenSet{}, fmt.Errorf("missing refresh_token; re-authenticate")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.clientID()},
		"refresh_token": {tokens.RefreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return TokenSet{}, fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var tr tokenResponse
	_ = json.Unmarshal(body, &tr)
	if resp.StatusCode != http.StatusOK || tr.AccessToken == "" {
		msg := tr.ErrorDesc
		if msg == "" {
			msg = tr.Error
		}
		if msg == "" {
			msg = truncate(string(body), 200)
		}
		return TokenSet{}, fmt.Errorf("refresh token: status %d: %s", resp.StatusCode, msg)
	}
	out := c.tokenSetFromResponse(tr, endpoint)
	if out.RefreshToken == "" {
		out.RefreshToken = tokens.RefreshToken
	}
	if tokens.APIBase != "" {
		out.APIBase = tokens.APIBase
	}
	return out, nil
}

// EnsureAccessToken loads tokens, refreshes if needed, and returns a usable access token.
func EnsureAccessToken(ctx context.Context, store TokenStore, client *OAuthClient) (TokenSet, error) {
	if client == nil {
		client = &OAuthClient{}
	}
	tokens, err := store.Load()
	if err != nil {
		return TokenSet{}, err
	}
	if tokens.AccessToken == "" && tokens.RefreshToken == "" {
		return TokenSet{}, fmt.Errorf("not signed in; use Sign in to xAI")
	}
	if !tokens.NeedsRefresh(client.now()) {
		return tokens, nil
	}
	refreshed, err := client.Refresh(ctx, tokens)
	if err != nil {
		// If still valid, return existing; else error.
		if tokens.Valid(client.now()) {
			return tokens, nil
		}
		return TokenSet{}, fmt.Errorf("refresh credentials: %w", err)
	}
	if err := store.Save(refreshed); err != nil {
		return TokenSet{}, err
	}
	return refreshed, nil
}

func (c *OAuthClient) tokenSetFromResponse(tr tokenResponse, tokenEndpoint string) TokenSet {
	ts := TokenSet{
		AccessToken:   tr.AccessToken,
		RefreshToken:  tr.RefreshToken,
		TokenType:     tr.TokenType,
		TokenEndpoint: tokenEndpoint,
		APIBase:       c.apiBase(),
	}
	if tr.TokenType == "" {
		ts.TokenType = "Bearer"
	}
	if tr.ExpiresIn > 0 {
		ts.ExpiresAt = c.now().UTC().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return ts
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// VerificationURL prefers the complete URI when present.
func (dc DeviceCodeResponse) VerificationURL() string {
	if dc.VerificationURIComplete != "" {
		return dc.VerificationURIComplete
	}
	return dc.VerificationURI
}
