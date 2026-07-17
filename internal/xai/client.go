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
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the body for POST /chat/completions.
type ChatRequest struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Stream          bool      `json:"stream,omitempty"`
}

// ChatResponse is a non-streaming completion response.
type ChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
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

// AssistantText returns the first choice content.
func (cr ChatResponse) AssistantText() string {
	if len(cr.Choices) == 0 {
		return ""
	}
	return cr.Choices[0].Message.Content
}
