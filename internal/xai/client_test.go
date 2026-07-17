package xai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"deedles.dev/aiadventure/internal/xai"
)

func TestChatParsesToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req xai.ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
			return
		}
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "start_adventure" {
			t.Errorf("tools=%+v", req.Tools)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "c1",
			"model": "m",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "call_x",
								"type": "function",
								"function": map[string]string{
									"name":      "start_adventure",
									"arguments": `{"reason":"ready"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &xai.Client{
		HTTP:    srv.Client(),
		APIBase: srv.URL,
		TokenProvider: func(ctx context.Context) (string, error) {
			return "t", nil
		},
	}
	req := xai.BuildChatRequest("test-model", "", []xai.Message{
		{Role: "user", Content: "ready"},
	})
	req.Tools = []xai.Tool{{
		Type: "function",
		Function: xai.ToolFunction{
			Name:        "start_adventure",
			Description: "begin",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}}
	resp, err := c.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.HasToolCalls() {
		t.Fatal("expected tool calls")
	}
	tc := resp.ToolCalls()
	if len(tc) != 1 || tc[0].ID != "call_x" || tc[0].Function.Name != "start_adventure" {
		t.Fatalf("tool_calls=%+v", tc)
	}
	tr := xai.ToolResultMessage(tc[0].ID, `{"ok":true}`)
	if tr.Role != "tool" || tr.ToolCallID != "call_x" || tr.Content != `{"ok":true}` {
		t.Fatalf("tool result=%+v", tr)
	}
}

func TestChatAssistantTextWithoutTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "c1",
			"model": "m",
			"choices": []map[string]any{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": "Hello"},
					"finish_reason": "stop",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := &xai.Client{HTTP: srv.Client(), APIBase: srv.URL}
	resp, err := c.Chat(context.Background(), xai.BuildChatRequest("m", "", []xai.Message{
		{Role: "user", Content: "hi"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.AssistantText() != "Hello" {
		t.Fatalf("text=%q", resp.AssistantText())
	}
	if resp.HasToolCalls() {
		t.Fatal("unexpected tool calls")
	}
}
