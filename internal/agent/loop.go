package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DefaultSystem tells the model what it is operating and where the line
// between it and the person lies.
const DefaultSystem = `你是 BiblioCell 的内置助手，通过工具直接操作这个人的单细胞文献库（正式数据库）。
规则：
- 不了解库的结构时，先调用 describe_library。
- 收录判断（screening）和核实是人的决定：你只能通过 proposedScreening 提建议，用 add_evidence 记录依据。
- 修改数据前先用查询工具确认对象存在；批量导入先 dryRun。
- 用中文简洁回答，说明你做了什么、改了哪些记录；不要编造工具没有返回的内容。`

// Event reports progress of one run, for the window to show as it happens.
type Event struct {
	// Type is step, tool_call, tool_result, or done.
	Type     string    `json:"type"`
	Step     int       `json:"step"`
	ToolCall *ToolCall `json:"toolCall,omitempty"`
	// Output is a tool result (tool_result) or the final text (done).
	Output  string `json:"output,omitempty"`
	IsError bool   `json:"isError,omitempty"`
}

// Result is what a run produced: the transcript grown by every assistant and
// tool turn, and the last assistant text.
type Result struct {
	Messages []Message `json:"messages"`
	Text     string    `json:"text"`
	Steps    int       `json:"steps"`
	// StopReason is end_turn, max_tokens, refusal, or max_steps.
	StopReason string `json:"stopReason"`
	Usage      Usage  `json:"usage"`
}

// Options tunes one run.
type Options struct {
	System   string
	MaxSteps int
	// OnEvent, when set, sees every step as it happens.
	OnEvent func(Event)
}

// Run is the loop: ask the model, run the tools it asks for, repeat until it
// answers in words or the step budget ends. Messages must end with a user turn.
func Run(ctx context.Context, provider Provider, tools Tools, messages []Message, options Options) (Result, error) {
	if len(messages) == 0 || messages[len(messages)-1].Role != "user" {
		return Result{}, errors.New("the conversation must end with a user message")
	}
	if options.System == "" {
		options.System = DefaultSystem
	}
	if options.MaxSteps <= 0 {
		options.MaxSteps = DefaultMaxSteps
	}
	emit := func(event Event) {
		if options.OnEvent != nil {
			options.OnEvent(event)
		}
	}
	defs, err := tools.List(ctx)
	if err != nil {
		return Result{}, err
	}
	transcript := append([]Message(nil), messages...)
	result := Result{StopReason: "max_steps"}
	for step := 1; step <= options.MaxSteps; step++ {
		result.Steps = step
		emit(Event{Type: "step", Step: step})
		response, err := provider.Complete(ctx, Request{System: options.System, Messages: transcript, Tools: defs})
		if err != nil {
			result.Messages = transcript
			return result, fmt.Errorf("step %d: %w", step, err)
		}
		result.Usage.Add(response.Usage)
		transcript = append(transcript, response.Message)
		result.Text = response.Message.Content
		if len(response.Message.ToolCalls) == 0 {
			result.StopReason = response.StopReason
			if result.StopReason == "" || result.StopReason == "tool_use" {
				result.StopReason = "end_turn"
			}
			break
		}
		for _, call := range response.Message.ToolCalls {
			call := call
			emit(Event{Type: "tool_call", Step: step, ToolCall: &call})
			output, isError, err := tools.Call(ctx, call.Name, json.RawMessage(call.Arguments))
			if err != nil {
				result.Messages = transcript
				return result, fmt.Errorf("step %d, tool %s: %w", step, call.Name, err)
			}
			transcript = append(transcript, Message{Role: "tool", ToolCallID: call.ID, ToolName: call.Name, Content: output, IsError: isError})
			emit(Event{Type: "tool_result", Step: step, ToolCall: &call, Output: output, IsError: isError})
		}
	}
	if result.StopReason == "max_steps" {
		result.Text = strings.TrimSpace(result.Text + "\n\n（已达到单次运行的步数上限，未完成的部分可以继续对话。）")
	}
	result.Messages = transcript
	emit(Event{Type: "done", Step: result.Steps, Output: result.Text})
	return result, nil
}
