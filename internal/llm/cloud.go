package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/user/ai-sh/internal/config"
)

// cloud calls a hosted model over the OpenAI chat-completions protocol, which
// every provider in config.Presets speaks.
type cloud struct {
	cfg    config.Config
	client *http.Client
}

func newCloud(cfg config.Config) (Provider, error) {
	return &cloud{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *cloud) Describe() string {
	label := c.cfg.Provider
	if preset, ok := config.Presets[c.cfg.Provider]; ok && preset.Label != "" {
		label = preset.Label
	}
	return label + ": " + c.cfg.Model
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Generate posts the conversation to the configured endpoint. Sampling matches
// the local provider: temperature 0.1, 100 token ceiling. Prior turns travel
// as real chat messages, so buildSystemPrompt gets nothing to fold in.
func (c *cloud) Generate(messages []Message) (string, error) {
	if _, _, err := currentInstruction(messages); err != nil {
		return "", err
	}

	chat := make([]chatMessage, 0, len(messages)+1)
	chat = append(chat, chatMessage{Role: "system", Content: buildSystemPrompt(nil)})
	for _, msg := range messages {
		chat = append(chat, chatMessage{Role: msg.Role, Content: msg.Content})
	}

	body, err := json.Marshal(chatRequest{
		Model:       c.cfg.Model,
		Messages:    chat,
		Temperature: 0.1,
		MaxTokens:   100,
	})
	if err != nil {
		return "", err
	}

	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling %s: %w", c.cfg.Provider, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading %s response: %w", c.cfg.Provider, err)
	}

	var parsed chatResponse
	// A non-JSON body (proxy error page, HTML) leaves parsed empty; the status
	// check below still produces a useful message.
	_ = json.Unmarshal(raw, &parsed)

	if resp.StatusCode != http.StatusOK {
		if parsed.Error.Message != "" {
			return "", fmt.Errorf("%s returned %s: %s", c.cfg.Provider, resp.Status, parsed.Error.Message)
		}
		return "", fmt.Errorf("%s returned %s: %s", c.cfg.Provider, resp.Status, truncate(strings.TrimSpace(string(raw)), 200))
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("%s returned no choices", c.cfg.Provider)
	}

	return stripMarkdown(strings.TrimSpace(parsed.Choices[0].Message.Content)), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
