package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Message is a chat message for the completions API.
// Content may be empty when ToolCalls is set (assistant tool-call turns).
// ToolCallID is set for role "tool" result messages.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is a model-requested function invocation.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the function name and JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is a function the model may call (OpenAI-compatible chat completions shape).
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a callable function and its JSON Schema parameters.
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Parameters is a JSON Schema object (typically map[string]any).
	Parameters any `json:"parameters,omitempty"`
}

// ChatRequest is the body for POST /chat/completions.
type ChatRequest struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Stream          bool      `json:"stream,omitempty"`
	Tools           []Tool    `json:"tools,omitempty"`
	// ToolChoice is "auto", "none", "required", or a forced-function object.
	ToolChoice any `json:"tool_choice,omitempty"`
}

// ChatChoice is one completion alternative.
type ChatChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// ChatResponse is a non-streaming completion response.
type ChatResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Client calls the xAI chat API with a bearer token.
type Client struct {
	HTTP    *http.Client
	APIBase string
	// TokenProvider returns a current access token.
	TokenProvider func(ctx context.Context) (string, error)
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) base() string {
	if c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return DefaultAPIBase
}

// BuildChatRequest constructs a request with model/effort rules applied.
func BuildChatRequest(modelID, effort string, messages []Message) ChatRequest {
	req := ChatRequest{
		Model:    modelID,
		Messages: messages,
	}
	if e := ResolveEffort(modelID, effort); e != "" {
		req.ReasoningEffort = e
	}
	return req
}

// Chat sends a non-streaming chat completion request.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	token := ""
	if c.TokenProvider != nil {
		var err error
		token, err = c.TokenProvider(ctx)
		if err != nil {
			return ChatResponse{}, fmt.Errorf("token: %w", err)
		}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("chat: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var cr ChatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return ChatResponse{}, fmt.Errorf("chat parse: %w (body: %s)", err, truncate(string(raw), 200))
	}
	if resp.StatusCode != http.StatusOK {
		msg := ""
		if cr.Error != nil {
			msg = cr.Error.Message
		}
		if msg == "" {
			msg = truncate(string(raw), 200)
		}
		return ChatResponse{}, fmt.Errorf("chat: status %d: %s", resp.StatusCode, msg)
	}
	if len(cr.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("chat: empty choices")
	}
	return cr, nil
}

// AssistantMessage returns the first choice message.
func (cr ChatResponse) AssistantMessage() Message {
	if len(cr.Choices) == 0 {
		return Message{}
	}
	return cr.Choices[0].Message
}

// AssistantText returns the first choice content.
func (cr ChatResponse) AssistantText() string {
	return cr.AssistantMessage().Content
}

// ToolCalls returns tool calls from the first choice, if any.
func (cr ChatResponse) ToolCalls() []ToolCall {
	return cr.AssistantMessage().ToolCalls
}

// HasToolCalls reports whether the first choice requested any tools.
func (cr ChatResponse) HasToolCalls() bool {
	return len(cr.ToolCalls()) > 0
}

// ToolResultMessage builds a role=tool message for a completed tool call.
func ToolResultMessage(callID, content string) Message {
	return Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    content,
	}
}
