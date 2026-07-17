package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"deedles.dev/aiadventure/internal/app"
	"deedles.dev/aiadventure/internal/config"
	"deedles.dev/aiadventure/internal/prompt"
	"deedles.dev/aiadventure/internal/session"
	"deedles.dev/aiadventure/internal/xai"
)

func TestSendUserMessageStartAdventureTool(t *testing.T) {
	var calls atomic.Int32
	var secondSystem string
	var firstHadTool bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req xai.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			http.Error(w, err.Error(), 500)
			return
		}
		n := calls.Add(1)
		switch n {
		case 1:
			if len(req.Tools) != 1 || req.Tools[0].Function.Name != prompt.ToolStartAdventure {
				t.Errorf("first call tools=%+v want start_adventure", req.Tools)
			}
			firstHadTool = len(req.Tools) == 1
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "t1",
				"model": "m",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]any{
							"role":    "assistant",
							"content": "",
							"tool_calls": []map[string]any{
								{
									"id":   "call_1",
									"type": "function",
									"function": map[string]string{
										"name":      prompt.ToolStartAdventure,
										"arguments": `{"reason":"player approved setup"}`,
									},
								},
							},
						},
						"finish_reason": "tool_calls",
					},
				},
			})
		case 2:
			if len(req.Messages) > 0 {
				secondSystem = req.Messages[0].Content
			}
			if len(req.Tools) != 0 {
				t.Errorf("second call should have no tools after phase switch, got %d", len(req.Tools))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":    "t2",
				"model": "m",
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]string{
							"role":    "assistant",
							"content": "You stand at the edge of a misty cliff…",
						},
						"finish_reason": "stop",
					},
				},
			})
		default:
			t.Errorf("unexpected call #%d", n)
			http.Error(w, "too many calls", 500)
		}
	}))
	t.Cleanup(srv.Close)

	deps := testChatDeps(t, srv)
	s := session.New("Quest", "test-model", "")
	_, _ = s.Append(session.RoleUser, "Fantasy coastal kingdom")
	_, _ = s.Append(session.RoleAssistant, "Sounds fun — any house rules?")

	if err := deps.SendUserMessage(context.Background(), s, "The setup is good"); err != nil {
		t.Fatal(err)
	}
	if !firstHadTool {
		t.Fatal("expected start_adventure tool on brainstorm call")
	}
	if s.Phase != session.PhaseAdventure {
		t.Fatalf("phase=%q want adventure after tool call", s.Phase)
	}
	if !strings.Contains(secondSystem, "narrator") {
		t.Fatalf("second call should use adventure system, got: %q", truncate(secondSystem, 120))
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d want 2", calls.Load())
	}
	path := s.ActivePath()
	// prior 2 turns + user ready + assistant opening
	if len(path) != 4 {
		t.Fatalf("path len=%d want 4 (tool turns not stored)", len(path))
	}
	if path[len(path)-1].Role != session.RoleAssistant || !strings.Contains(path[len(path)-1].Content, "cliff") {
		t.Fatalf("last turn=%+v", path[len(path)-1])
	}
}

func TestSendUserMessageKeepsBrainstormWithoutTool(t *testing.T) {
	var sawTools int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req xai.ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sawTools = len(req.Tools)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "t1",
			"model": "m",
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": "More about factions?"},
					"finish_reason": "stop",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	deps := testChatDeps(t, srv)
	s := session.New("Quest", "test-model", "")
	if err := deps.SendUserMessage(context.Background(), s, "I want a dark fantasy coastal kingdom"); err != nil {
		t.Fatal(err)
	}
	if s.Phase != session.PhaseBrainstorm {
		t.Fatalf("phase=%q want brainstorm", s.Phase)
	}
	if sawTools != 1 {
		t.Fatalf("brainstorm should offer tools, got %d", sawTools)
	}
}

func TestSendUserMessageAdventurePhaseHasNoTools(t *testing.T) {
	var sawTools int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req xai.ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sawTools = len(req.Tools)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "t1",
			"model": "m",
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": "The door creaks open."},
					"finish_reason": "stop",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	deps := testChatDeps(t, srv)
	s := session.New("Quest", "test-model", "")
	_ = s.SetPhase(session.PhaseAdventure)
	if err := deps.SendUserMessage(context.Background(), s, "I open the door"); err != nil {
		t.Fatal(err)
	}
	if sawTools != 0 {
		t.Fatalf("adventure should not offer tools, got %d", sawTools)
	}
}

func testChatDeps(t *testing.T, srv *httptest.Server) *app.Deps {
	t.Helper()
	dir := t.TempDir()
	deps := app.NewDeps(config.Config{SessionsDir: dir, Model: "test-model"}, config.Paths{})
	deps.HTTP = &xai.Client{
		HTTP:    srv.Client(),
		APIBase: srv.URL,
		TokenProvider: func(ctx context.Context) (string, error) {
			return "tok", nil
		},
	}
	return deps
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
