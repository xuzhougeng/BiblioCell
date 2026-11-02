package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// openAIProvider speaks the Chat Completions API (POST /chat/completions),
// which most hosted and local model servers also implement.
type openAIProvider struct {
	config Config
	client *http.Client
}

type openAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIRequest struct {
	Model     string            `json:"model"`
	Messages  []json.RawMessage `json:"messages"`
	Tools     []openAITool      `json:"tools,omitempty"`
	MaxTokens int               `json:"max_tokens,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message      json.RawMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *openAIProvider) Protocol() string { return ProtocolOpenAI }

func (p *openAIProvider) Complete(ctx context.Context, request Request) (Response, error) {
	body := openAIRequest{Model: p.config.Model, MaxTokens: p.config.MaxTokens}
	for _, tool := range request.Tools {
		body.Tools = append(body.Tools, openAITool{Type: "function", Function: openAIFunction{
			Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema,
		}})
	}
	if request.System != "" {
		if err := appendMessage(&body.Messages, openAIMessage{Role: "system", Content: &request.System}); err != nil {
			return Response{}, err
		}
	}
	for _, message := range request.Messages {
		switch message.Role {
		case "user":
			content := message.Content
			if err := appendMessage(&body.Messages, openAIMessage{Role: "user", Content: &content}); err != nil {
				return Response{}, err
			}
		case "tool":
			content := message.Content
			if err := appendMessage(&body.Messages, openAIMessage{Role: "tool", Content: &content, ToolCallID: message.ToolCallID}); err != nil {
				return Response{}, err
			}
		case "assistant":
			if message.Raw != "" && message.RawProtocol == ProtocolOpenAI && json.Valid([]byte(message.Raw)) {
				body.Messages = append(body.Messages, json.RawMessage(message.Raw))
				continue
			}
			encoded := openAIMessage{Role: "assistant"}
			if message.Content != "" || len(message.ToolCalls) == 0 {
				content := message.Content
				encoded.Content = &content
			}
			for _, call := range message.ToolCalls {
				var toolCall openAIToolCall
				toolCall.ID, toolCall.Type = call.ID, "function"
				toolCall.Function.Name = call.Name
				toolCall.Function.Arguments = string(argumentsJSON(call.Arguments))
				encoded.ToolCalls = append(encoded.ToolCalls, toolCall)
			}
			if err := appendMessage(&body.Messages, encoded); err != nil {
				return Response{}, err
			}
		default:
			return Response{}, fmt.Errorf("unsupported message role %q", message.Role)
		}
	}

	headers := map[string]string{}
	if p.config.APIKey != "" {
		headers["Authorization"] = "Bearer " + p.config.APIKey
	}
	var reply openAIResponse
	if err := postJSON(ctx, p.client, p.config.BaseURL+"/chat/completions", headers, body, &reply); err != nil {
		return Response{}, err
	}
	if len(reply.Choices) == 0 {
		return Response{}, fmt.Errorf("%s returned no choices", p.config.BaseURL)
	}
	choice := reply.Choices[0]
	var decoded openAIMessage
	if err := json.Unmarshal(choice.Message, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode assistant message: %w", err)
	}
	message := Message{Role: "assistant", Raw: string(choice.Message), RawProtocol: ProtocolOpenAI}
	if decoded.Content != nil {
		message.Content = *decoded.Content
	}
	for _, call := range decoded.ToolCalls {
		message.ToolCalls = append(message.ToolCalls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: string(argumentsJSON(call.Function.Arguments))})
	}
	response := Response{Message: message, StopReason: stopReason(choice.FinishReason, len(message.ToolCalls) > 0)}
	response.Usage.InputTokens = reply.Usage.PromptTokens
	response.Usage.OutputTokens = reply.Usage.CompletionTokens
	if response.StopReason == "refusal" {
		response.Message.Content = refusalText("content_filter", "")
	}
	return response, nil
}

func appendMessage(messages *[]json.RawMessage, message openAIMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	*messages = append(*messages, data)
	return nil
}

// stopReason maps finish_reason onto the Messages API vocabulary the loop uses.
func stopReason(finish string, hasToolCalls bool) string {
	switch {
	case hasToolCalls || finish == "tool_calls":
		return "tool_use"
	case finish == "length":
		return "max_tokens"
	case finish == "content_filter":
		return "refusal"
	}
	return "end_turn"
}
