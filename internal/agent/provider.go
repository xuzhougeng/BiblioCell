package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Protocols the loop can speak. Each is a wire format, not a vendor: any
// service that serves the same endpoints works through a custom base URL.
const (
	ProtocolAnthropic = "anthropic"
	ProtocolOpenAI    = "openai"
)

// Config is what the person fills in to reach a model.
type Config struct {
	// Protocol is anthropic (Messages API) or openai (Chat Completions).
	Protocol string `json:"protocol"`
	// BaseURL is the service root; empty means the protocol's public endpoint.
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
	// MaxTokens caps one completion; 0 uses the protocol default.
	MaxTokens int `json:"maxTokens"`
	// MaxSteps caps the completions of one run; 0 uses DefaultMaxSteps.
	MaxSteps int `json:"maxSteps"`
}

// Defaults for a blank configuration.
const (
	DefaultAnthropicBaseURL = "https://api.anthropic.com"
	DefaultOpenAIBaseURL    = "https://api.openai.com/v1"
	DefaultAnthropicModel   = "claude-opus-5-5"
	DefaultMaxTokens        = 16000
	DefaultMaxSteps         = 20
	requestTimeout          = 5 * time.Minute
)

// Request is one completion call.
type Request struct {
	System   string
	Messages []Message
	Tools    []ToolDef
}

// Response is what the model answered.
type Response struct {
	Message Message
	// StopReason is end_turn, tool_use, max_tokens, or refusal.
	StopReason string
	Usage      Usage
}

// Provider completes a conversation over one protocol.
type Provider interface {
	Protocol() string
	Complete(ctx context.Context, request Request) (Response, error)
}

// Trim cleans what the person typed without filling in defaults, so a blank
// section stays blank in the settings file.
func Trim(config Config) Config {
	config.Protocol = strings.ToLower(strings.TrimSpace(config.Protocol))
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = strings.TrimSpace(config.Model)
	return config
}

// Normalize trims and fills protocol defaults.
func Normalize(config Config) Config {
	config = Trim(config)
	if config.Protocol == "" {
		config.Protocol = ProtocolAnthropic
	}
	if config.BaseURL == "" {
		switch config.Protocol {
		case ProtocolAnthropic:
			config.BaseURL = DefaultAnthropicBaseURL
		case ProtocolOpenAI:
			config.BaseURL = DefaultOpenAIBaseURL
		}
	}
	if config.Model == "" && config.Protocol == ProtocolAnthropic {
		config.Model = DefaultAnthropicModel
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = DefaultMaxSteps
	}
	return config
}

// Validate rejects a configuration the loop cannot use.
func Validate(config Config) error {
	config = Normalize(config)
	switch config.Protocol {
	case ProtocolAnthropic, ProtocolOpenAI:
	default:
		return fmt.Errorf("unsupported AI protocol %q: use anthropic or openai", config.Protocol)
	}
	if !strings.HasPrefix(config.BaseURL, "http://") && !strings.HasPrefix(config.BaseURL, "https://") {
		return errors.New("AI base URL must start with http:// or https://")
	}
	if config.Model == "" {
		return errors.New("AI model name is required")
	}
	if config.MaxTokens < 0 || config.MaxTokens > 1_000_000 {
		return errors.New("AI max tokens must be between 0 and 1000000")
	}
	if config.MaxSteps > 200 {
		return errors.New("AI max steps must be at most 200")
	}
	for _, value := range []string{config.APIKey, config.Model} {
		if strings.ContainsFunc(value, func(r rune) bool { return r < 32 || r == 127 }) {
			return errors.New("AI settings cannot contain control characters")
		}
	}
	return nil
}

// NewProvider builds the client for a configuration.
func NewProvider(config Config) (Provider, error) {
	if err := Validate(config); err != nil {
		return nil, err
	}
	config = Normalize(config)
	client := &http.Client{Timeout: requestTimeout}
	switch config.Protocol {
	case ProtocolAnthropic:
		return &anthropicProvider{config: config, client: client}, nil
	case ProtocolOpenAI:
		return &openAIProvider{config: config, client: client}, nil
	}
	return nil, fmt.Errorf("unsupported AI protocol %q", config.Protocol)
}

// postJSON sends a request body and decodes the reply, turning non-2xx
// answers into errors that carry the service's own message.
func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body any, reply any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("reach %s: %w", url, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("read reply from %s: %w", url, err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("%s answered %s: %s", url, response.Status, errorDetail(data))
	}
	if err := json.Unmarshal(data, reply); err != nil {
		return fmt.Errorf("decode reply from %s: %w", url, err)
	}
	return nil
}

// errorDetail extracts the message both protocols put under "error".
func errorDetail(data []byte) string {
	var body struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && len(body.Error) > 0 {
		var detail struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body.Error, &detail) == nil && detail.Message != "" {
			return detail.Message
		}
		var text string
		if json.Unmarshal(body.Error, &text) == nil && text != "" {
			return text
		}
	}
	text := strings.TrimSpace(string(data))
	if len(text) > 500 {
		text = text[:500] + "…"
	}
	if text == "" {
		return "empty response body"
	}
	return text
}

// argumentsJSON returns the call's arguments as a JSON object, defaulting to
// an empty one so a tool with no parameters still receives valid input.
func argumentsJSON(arguments string) json.RawMessage {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(trimmed)
}
