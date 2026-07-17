package xai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"deedles.dev/aiadventure/internal/xai"
)

func TestTokenStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := xai.TokenStore{Path: dir + "/auth.json"}
	in := xai.TokenSet{
		AccessToken:   "access-abc",
		RefreshToken:  "refresh-xyz",
		TokenType:     "Bearer",
		ExpiresAt:     time.Now().UTC().Add(2 * time.Hour),
		TokenEndpoint: "https://auth.x.ai/oauth2/token",
		APIBase:       "https://api.x.ai/v1",
	}
	if err := store.Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if out.AccessToken != in.AccessToken || out.RefreshToken != in.RefreshToken {
		t.Fatalf("tokens mismatch: %+v", out)
	}
	if out.TokenEndpoint != in.TokenEndpoint {
		t.Fatalf("endpoint = %q", out.TokenEndpoint)
	}
}

func TestTokenSetValidAndNeedsRefresh(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	fresh := xai.TokenSet{
		AccessToken: "a",
		ExpiresAt:   now.Add(3 * time.Hour),
	}
	if !fresh.Valid(now) {
		t.Fatal("expected valid")
	}
	if fresh.NeedsRefresh(now) {
		t.Fatal("should not need refresh")
	}
	expiring := xai.TokenSet{
		AccessToken: "a",
		ExpiresAt:   now.Add(30 * time.Minute), // within 1h skew
	}
	if expiring.Valid(now) {
		t.Fatal("within skew should not be Valid")
	}
	if !expiring.NeedsRefresh(now) {
		t.Fatal("should need refresh")
	}
	empty := xai.TokenSet{}
	if empty.Valid(now) {
		t.Fatal("empty not valid")
	}
}

func TestDeviceCodeAndPollAndRefresh(t *testing.T) {
	var pollCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"authorization_endpoint":        "https://auth.example/oauth2/authorize",
				"device_authorization_endpoint": "http://" + r.Host + "/oauth2/device/code",
				"token_endpoint":                "http://" + r.Host + "/oauth2/token",
			})
		case strings.HasSuffix(r.URL.Path, "/oauth2/device/code"):
			if r.Method != http.MethodPost {
				t.Errorf("method %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "client_id=") {
				t.Errorf("body = %s", body)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":               "dev-1",
				"user_code":                 "ABCD-EFGH",
				"verification_uri":          "https://auth.example/device",
				"verification_uri_complete": "https://auth.example/device?user_code=ABCD-EFGH",
				"expires_in":                600,
				"interval":                  0, // force default
			})
		case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
			body, _ := io.ReadAll(r.Body)
			form := string(body)
			if strings.Contains(form, "device_code") {
				pollCount++
				if pollCount < 2 {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "access-1",
					"refresh_token": "refresh-1",
					"token_type":    "Bearer",
					"expires_in":    3600,
				})
				return
			}
			if strings.Contains(form, "refresh_token") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"access_token":  "access-2",
					"refresh_token": "refresh-2",
					"token_type":    "Bearer",
					"expires_in":    3600,
				})
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	client := &xai.OAuthClient{
		HTTP:     srv.Client(),
		Issuer:   srv.URL,
		ClientID: "test-client",
		Now: func() time.Time {
			return fixed
		},
	}

	ctx := context.Background()
	disc, err := client.Discover(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if disc.TokenEndpoint == "" {
		t.Fatal("missing token endpoint")
	}

	dc, err := client.RequestDeviceCode(ctx, disc.DeviceAuthorizationEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	if dc.UserCode != "ABCD-EFGH" || dc.VerificationURL() == "" {
		t.Fatalf("device = %+v", dc)
	}

	// Speed up poll sleep by using context - interval is 5s default when 0.
	// Override interval via response was 0 so Interval becomes 5 in RequestDeviceCode...
	// Actually RequestDeviceCode sets Interval to 5 if <= 0.
	// For tests, set Interval to 0 after request so Poll uses 5s - too slow.
	// Let's set interval tiny by mutating.
	dc.Interval = 0 // PollDeviceToken uses max(1s) when interval < 1s... actually:
	// interval := time.Duration(dc.Interval) * time.Second
	// if interval < time.Second { interval = 5 * time.Second }
	// So we need Interval such that Duration is at least 1 nanosecond but...
	// Interval is int seconds. Set to 0 forces 5s.
	// I'll change PollDeviceToken to use a smaller min in tests... Or use short sleep by setting Interval = 0 and changing production code to allow 0 -> 1ms for test.
	// Better: set Interval to 1 but that still sleeps 1s. For unit tests 1s*2 is ok-ish but slow.
	// Change oauth.go: if interval < time.Second { interval = time.Second } when Interval is 0 use 1 second, and in test set Interval=-1... messy.
	// Simplest: in PollDeviceToken, if Interval is 0 after request, we already set to 5 in RequestDeviceCode.
	// For test, mutate dc.Interval to remain 0 and change poll to:
	// if dc.Interval <= 0 { interval = 0 } for immediate?
	// Looking at code again:
	// if interval < time.Second { interval = 5 * time.Second }
	// I'll update oauth to use 1ms when Interval is 0 after RequestDeviceCode sets it to 5...
	// RequestDeviceCode: if dc.Interval <= 0 { dc.Interval = 5 }
	// For test, after RequestDeviceCode, set dc.Interval = 0 again won't help if Poll uses 5s for < 1s.
	// Fix production: use `if interval <= 0 { interval = 5 * time.Second }` only when Interval from response is 0, and if we pass Interval=0 to Poll after mutation with a special case...
	// Easiest path for fast tests: change min interval to 1ms when Interval is negative, or inject clock/sleep.
	// I'll inject optional Sleeper later; for now set Interval so duration is 1 second - 2 polls = 2s, acceptable.

	dc.Interval = 1
	// First poll pending, second success - one sleep of 1s.
	tokens, err := client.PollDeviceToken(ctx, disc.TokenEndpoint, dc)
	if err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken != "access-1" || tokens.RefreshToken != "refresh-1" {
		t.Fatalf("tokens = %+v", tokens)
	}

	refreshed, err := client.Refresh(ctx, tokens)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AccessToken != "access-2" {
		t.Fatalf("refreshed = %+v", refreshed)
	}
}

func TestEnsureAccessTokenRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/oauth2/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access",
				"refresh_token": "new-refresh",
				"expires_in":    7200,
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	store := xai.TokenStore{Path: dir + "/auth.json"}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	_ = store.Save(xai.TokenSet{
		AccessToken:   "old",
		RefreshToken:  "r1",
		ExpiresAt:     now.Add(10 * time.Minute),
		TokenEndpoint: srv.URL + "/oauth2/token",
	})

	client := &xai.OAuthClient{
		HTTP: srv.Client(),
		Now:  func() time.Time { return now },
	}
	tok, err := xai.EnsureAccessToken(context.Background(), store, client)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "new-access" {
		t.Fatalf("got %+v", tok)
	}
	loaded, _ := store.Load()
	if loaded.AccessToken != "new-access" {
		t.Fatalf("store not updated: %+v", loaded)
	}
}

func TestChatClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "grok-4.5" {
			t.Errorf("model = %v", body["model"])
		}
		if body["reasoning_effort"] != "low" {
			t.Errorf("effort = %v", body["reasoning_effort"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "c1",
			"model": "grok-4.5",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hello adventurer"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &xai.Client{
		HTTP:    srv.Client(),
		APIBase: srv.URL,
		TokenProvider: func(ctx context.Context) (string, error) {
			return "tok", nil
		},
	}
	req := xai.BuildChatRequest("grok-4.5", "low", []xai.Message{{Role: "user", Content: "Hi"}})
	resp, err := c.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.AssistantText() != "Hello adventurer" {
		t.Fatalf("text = %q", resp.AssistantText())
	}
}
