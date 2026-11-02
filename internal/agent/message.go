// Package agent runs a minimal tool-calling loop over the library's MCP
// tools, against whichever chat API protocol the person configured.
package agent

// Message is one turn of a conversation, in a form every protocol can carry.
// The frontend keeps the transcript and sends it back whole, so the Go side
// holds no session state.
type Message struct {
	// Role is user, assistant, or tool.
	Role string `json:"role"`
	// Content is the text of a user or assistant turn, or a tool's output.
	Content string `json:"content"`
	// Thinking is the reasoning summary a model chose to show, for display only.
	Thinking string `json:"thinking,omitempty"`
	// ToolCalls are the calls an assistant turn asked for.
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	// ToolCallID names the call a tool turn answers.
	ToolCallID string `json:"toolCallId,omitempty"`
	// ToolName repeats the called tool's name on a tool turn, for display.
	ToolName string `json:"toolName,omitempty"`
	// IsError marks a tool turn whose call failed.
	IsError bool `json:"isError,omitempty"`
	// Raw is the provider's own encoding of an assistant turn (JSON), replayed
	// verbatim so protocol-specific blocks such as reasoning survive the loop.
	// RawProtocol says which protocol wrote it; another protocol ignores it.
	Raw         string `json:"raw,omitempty"`
	RawProtocol string `json:"rawProtocol,omitempty"`
}

// ToolCall is a request from the model to run one tool.
type ToolCall struct {
	ID string `json:"id"`
	// Name is the MCP tool name.
	Name string `json:"name"`
	// Arguments is the JSON object the model supplied.
	Arguments string `json:"arguments"`
}

// ToolDef describes a tool to the model.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// InputSchema is a JSON Schema object.
	InputSchema any `json:"inputSchema"`
}

// Usage counts the tokens one completion consumed.
type Usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

// Add accumulates another completion's usage.
func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
}
