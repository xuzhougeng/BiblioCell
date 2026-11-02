package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// anthropicProvider speaks the Anthropic Messages API (POST /v1/messages).
// The request stays minimal on purpose: no thinking or sampling parameters,
// so current models run with their defaults and any compatible gateway
// accepts it.
type anthropicProvider struct {
	config Config
	client *http.Client
}

const anthropicVersion = "2023-06-01"

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicBlock struct {
	Type string `json:"type"`
	// text, and thinking's summary
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicRequest struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	System       string             `json:"system,omitempty"`
	Messages     []anthropicMessage `json:"messages"`
	Tools        []anthropicTool    `json:"tools,omitempty"`
	OutputConfig *anthropicOutput   `json:"output_config,omitempty"`
}

type anthropicOutput struct {
	Effort string `json:"effort"`
}

type anthropicResponse struct {
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
}

func (p *anthropicProvider) Protocol() string { return ProtocolAnthropic }

func (p *anthropicProvider) Complete(ctx context.Context, request Request) (Response, error) {
	body := anthropicRequest{Model: p.config.Model, MaxTokens: p.config.MaxTokens, System: request.System}
	if body.MaxTokens <= 0 {
		body.MaxTokens = DefaultMaxTokens
	}
	if p.config.Effort != "" {
		body.OutputConfig = &anthropicOutput{Effort: p.config.Effort}
	}
	for _, tool := range request.Tools {
		body.Tools = append(body.Tools, anthropicTool{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	messages, err := p.encode(request.Messages)
	if err != nil {
		return Response{}, err
	}
	body.Messages = messages

	headers := map[string]string{"anthropic-version": anthropicVersion}
	if p.config.APIKey != "" {
		headers["x-api-key"] = p.config.APIKey
	}
	var reply anthropicResponse
	if err := postJSON(ctx, p.client, p.config.BaseURL+"/v1/messages", headers, body, &reply); err != nil {
		return Response{}, err
	}
	return p.decode(reply)
}

// encode turns the neutral transcript into Messages API turns. Tool results
// of one assistant turn are gathered into a single user turn, as the API
// requires.
func (p *anthropicProvider) encode(messages []Message) ([]anthropicMessage, error) {
	var out []anthropicMessage
	var pendingResults []anthropicBlock
	flushResults := func() error {
		if len(pendingResults) == 0 {
			return nil
		}
		content, err := json.Marshal(pendingResults)
		if err != nil {
			return err
		}
		out = append(out, anthropicMessage{Role: "user", Content: content})
		pendingResults = nil
		return nil
	}
	for _, message := range messages {
		switch message.Role {
		case "tool":
			pendingResults = append(pendingResults, anthropicBlock{
				Type: "tool_result", ToolUseID: message.ToolCallID, Content: message.Content, IsError: message.IsError,
			})
			continue
		case "user":
			if err := flushResults(); err != nil {
				return nil, err
			}
			content, err := json.Marshal([]anthropicBlock{{Type: "text", Text: message.Content}})
			if err != nil {
				return nil, err
			}
			out = append(out, anthropicMessage{Role: "user", Content: content})
		case "assistant":
			if err := flushResults(); err != nil {
				return nil, err
			}
			if message.Raw != "" && message.RawProtocol == ProtocolAnthropic && json.Valid([]byte(message.Raw)) {
				out = append(out, anthropicMessage{Role: "assistant", Content: json.RawMessage(message.Raw)})
				continue
			}
			var blocks []anthropicBlock
			if message.Content != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: message.Content})
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: argumentsJSON(call.Arguments)})
			}
			if len(blocks) == 0 {
				continue
			}
			content, err := json.Marshal(blocks)
			if err != nil {
				return nil, err
			}
			out = append(out, anthropicMessage{Role: "assistant", Content: content})
		default:
			return nil, fmt.Errorf("unsupported message role %q", message.Role)
		}
	}
	if err := flushResults(); err != nil {
		return nil, err
	}
	return out, nil
}

// decode reads the assistant turn. The raw content is kept so thinking and
// other blocks go back unchanged on the next call.
func (p *anthropicProvider) decode(reply anthropicResponse) (Response, error) {
	var blocks []anthropicBlock
	if err := json.Unmarshal(reply.Content, &blocks); err != nil {
		return Response{}, fmt.Errorf("decode content blocks: %w", err)
	}
	message := Message{Role: "assistant", Raw: string(reply.Content), RawProtocol: ProtocolAnthropic}
	var text, thinking []string
	for _, block := range blocks {
		switch block.Type {
		case "thinking":
			if strings.TrimSpace(block.Thinking) != "" {
				thinking = append(thinking, block.Thinking)
			}
		case "text":
			text = append(text, block.Text)
		case "tool_use":
			message.ToolCalls = append(message.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: string(argumentsJSON(string(block.Input)))})
		}
	}
	message.Content = strings.Join(text, "\n")
	message.Thinking = strings.Join(thinking, "\n\n")
	response := Response{Message: message, StopReason: reply.StopReason}
	response.Usage.InputTokens = reply.Usage.InputTokens
	response.Usage.OutputTokens = reply.Usage.OutputTokens
	if reply.StopReason == "refusal" && reply.StopDetails != nil {
		response.Message.Content = strings.TrimSpace(response.Message.Content + "\n" + refusalText(reply.StopDetails.Category, reply.StopDetails.Explanation))
	}
	return response, nil
}

func refusalText(category, explanation string) string {
	text := "模型拒绝了这次请求"
	if category != "" {
		text += "（" + category + "）"
	}
	if explanation != "" {
		text += "：" + explanation
	}
	return text
}
