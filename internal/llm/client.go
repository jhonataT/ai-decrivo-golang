package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Finding struct {
	Line     int    `json:"line"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

type Client struct {
	apiKey string
	model  string
	http   *http.Client
}

func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  "claude-sonnet-5",
		http:   &http.Client{Timeout: 3 * time.Minute},
	}
}

type request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type response struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) ReviewFile(ctx context.Context, path, patch string) ([]Finding, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	body, err := json.Marshal(request{
		Model:     c.model,
		MaxTokens: 2000,
		System:    systemPrompt,
		Messages: []message{{
			Role:    "user",
			Content: fmt.Sprintf("File: %s\n\nDiff:\n```diff\n%s\n```", path, patch),
		}},
	})

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))

	if err != nil {
		return nil, err
	}

	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)

	if err != nil {
		return nil, fmt.Errorf("chamada à API: %w", err)
	}
	defer resp.Body.Close()

	var parsed response

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decodificar resposta: %w", err)
	}

	if parsed.Error != nil {
		return nil, fmt.Errorf("api: %s", parsed.Error.Message)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API status response %d", resp.StatusCode)
	}

	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	return parseFindings(text.String())
}

func parseFindings(s string) ([]Finding, error) {
	start := strings.Index(s, "[")
	end := strings.Index(s, "]")

	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("Response without a valid JSON: %.120s", s)
	}

	var findings []Finding
	if err := json.Unmarshal([]byte(s[start:end+1]), &findings); err != nil {
		return nil, fmt.Errorf("Invalid json: %w", err)
	}

	return findings, nil
}
