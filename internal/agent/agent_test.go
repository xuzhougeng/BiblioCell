package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bibliocell/internal/library"
)

type fakeTools struct {
	calls []string
}

func (f *fakeTools) List(context.Context) ([]ToolDef, error) {
	return []ToolDef{{Name: "echo", Description: "Echo", InputSchema: map[string]any{"type": "object"}}}, nil
}

func (f *fakeTools) Call(_ context.Context, name string, arguments json.RawMessage) (string, bool, error) {
	f.calls = append(f.calls, name+":"+string(arguments))
	if name == "boom" {
		return "no such tool", true, nil
	}
	return "echoed " + string(arguments), false, nil
}

// scriptedProvider answers with tool calls until the script runs out.
type scriptedProvider struct {
	responses []Response
	requests  []Request
}

func (s *scriptedProvider) Protocol() string { return "fake" }

func (s *scriptedProvider) Complete(_ context.Context, request Request) (Response, error) {
	s.requests = append(s.requests, request)
	if len(s.responses) == 0 {
		return Response{}, errors.New("script exhausted")
	}
	next := s.responses[0]
	s.responses = s.responses[1:]
	return next, nil
}

func TestRunLoopsThroughToolCalls(t *testing.T) {
	provider := &scriptedProvider{responses: []Response{
		{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Name: "echo", Arguments: `{"q":"a"}`}, {ID: "c2", Name: "boom", Arguments: "{}"}}}, StopReason: "tool_use"},
		{Message: Message{Role: "assistant", Content: "done"}, StopReason: "end_turn", Usage: Usage{InputTokens: 3, OutputTokens: 4}},
	}}
	tools := &fakeTools{}
	var events []string
	result, err := Run(context.Background(), provider, tools, []Message{{Role: "user", Content: "hi"}}, Options{OnEvent: func(e Event) { events = append(events, e.Type) }})
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "done" || result.Steps != 2 || result.StopReason != "end_turn" || result.Usage.OutputTokens != 4 {
		t.Fatalf("result = %+v", result)
	}
	if got := strings.Join(tools.calls, ","); got != `echo:{"q":"a"},boom:{}` {
		t.Fatalf("tool calls = %s", got)
	}
	// user, assistant(tool calls), tool, tool, assistant(final)
	if len(result.Messages) != 5 || result.Messages[2].Role != "tool" || result.Messages[2].ToolCallID != "c1" || !result.Messages[3].IsError {
		t.Fatalf("transcript = %+v", result.Messages)
	}
	if provider.requests[0].System != DefaultSystem || len(provider.requests[0].Tools) != 1 || len(provider.requests[1].Messages) != 4 {
		t.Fatalf("requests = %+v", provider.requests)
	}
	if got := strings.Join(events, ","); got != "step,tool_call,tool_result,tool_call,tool_result,step,done" {
		t.Fatalf("events = %s", got)
	}
}

func TestRunStopsAtMaxSteps(t *testing.T) {
	provider := &scriptedProvider{responses: []Response{
		{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "1", Name: "echo", Arguments: "{}"}}}},
		{Message: Message{Role: "assistant", ToolCalls: []ToolCall{{ID: "2", Name: "echo", Arguments: "{}"}}}},
		{Message: Message{Role: "assistant", Content: "never"}},
	}}
	result, err := Run(context.Background(), provider, &fakeTools{}, []Message{{Role: "user", Content: "go"}}, Options{MaxSteps: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != "max_steps" || result.Steps != 2 || len(provider.responses) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := Run(context.Background(), provider, &fakeTools{}, nil, Options{}); err == nil {
		t.Fatal("empty conversation accepted")
	}
}

func TestAnthropicProtocol(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "sk-test" || r.Header.Get("anthropic-version") == "" {
			http.Error(w, "bad request shape", http.StatusBadRequest)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			w.Write([]byte(`{"content":[{"type":"thinking","thinking":"Need the list.","signature":"sig"},{"type":"text","text":"Looking."},{"type":"tool_use","id":"tu_1","name":"list_papers","input":{"query":"rice"}}],"stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`))
			return
		}
		w.Write([]byte(`{"content":[{"type":"text","text":"Found 2."}],"stop_reason":"end_turn","usage":{"input_tokens":20,"output_tokens":3}}`))
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Protocol: ProtocolAnthropic, BaseURL: server.URL + "/", APIKey: "sk-test", Model: "claude-opus-5-5", Effort: "xhigh"})
	if err != nil {
		t.Fatal(err)
	}
	tools := []ToolDef{{Name: "list_papers", Description: "List", InputSchema: map[string]any{"type": "object"}}}
	first, err := provider.Complete(context.Background(), Request{System: "sys", Messages: []Message{{Role: "user", Content: "find rice"}}, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	if first.Message.Thinking != "Need the list." {
		t.Fatalf("thinking = %q", first.Message.Thinking)
	}
	if first.StopReason != "tool_use" || first.Message.Content != "Looking." || len(first.Message.ToolCalls) != 1 || first.Message.ToolCalls[0].Arguments != `{"query":"rice"}` || first.Usage.InputTokens != 10 {
		t.Fatalf("first = %+v", first)
	}
	if requests[0]["system"] != "sys" || requests[0]["model"] != "claude-opus-5-5" || requests[0]["max_tokens"].(float64) != DefaultMaxTokens {
		t.Fatalf("request = %v", requests[0])
	}
	if _, ok := requests[0]["thinking"]; ok {
		t.Fatal("thinking parameter sent")
	}
	if effort := requests[0]["output_config"].(map[string]any)["effort"]; effort != "xhigh" {
		t.Fatalf("effort = %v", effort)
	}
	if tool := requests[0]["tools"].([]any)[0].(map[string]any); tool["input_schema"] == nil || tool["name"] != "list_papers" {
		t.Fatalf("tool = %v", tool)
	}

	transcript := []Message{{Role: "user", Content: "find rice"}, first.Message, {Role: "tool", ToolCallID: "tu_1", Content: `{"total":2}`}}
	second, err := provider.Complete(context.Background(), Request{Messages: transcript, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	if second.StopReason != "end_turn" || second.Message.Content != "Found 2." {
		t.Fatalf("second = %+v", second)
	}
	messages := requests[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %v", messages)
	}
	assistant := messages[1].(map[string]any)
	blocks := assistant["content"].([]any)
	if assistant["role"] != "assistant" || len(blocks) != 3 || blocks[0].(map[string]any)["type"] != "thinking" {
		t.Fatalf("assistant turn lost raw blocks: %v", assistant)
	}
	toolResult := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "tu_1" || toolResult["content"] != `{"total":2}` {
		t.Fatalf("tool result = %v", toolResult)
	}
}

func TestOpenAIProtocol(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(w, `{"error":{"message":"wrong path or key"}}`, http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests = append(requests, body)
		w.Header().Set("Content-Type", "application/json")
		if len(requests) == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"reasoning_content":"hmm","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_paper","arguments":"{\"id\":\"P1\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Here it is."},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":3}}`))
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Protocol: ProtocolOpenAI, BaseURL: server.URL + "/v1", APIKey: "sk-test", Model: "gpt-test", MaxTokens: 512, Effort: "max"})
	if err != nil {
		t.Fatal(err)
	}
	tools := []ToolDef{{Name: "get_paper", InputSchema: map[string]any{"type": "object"}}}
	first, err := provider.Complete(context.Background(), Request{System: "sys", Messages: []Message{{Role: "user", Content: "show P1"}}, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	if first.StopReason != "tool_use" || len(first.Message.ToolCalls) != 1 || first.Message.ToolCalls[0].Arguments != `{"id":"P1"}` || first.Usage.InputTokens != 7 {
		t.Fatalf("first = %+v", first)
	}
	if requests[0]["max_tokens"].(float64) != 512 || requests[0]["messages"].([]any)[0].(map[string]any)["role"] != "system" || requests[0]["reasoning_effort"] != "high" {
		t.Fatalf("request = %v", requests[0])
	}
	if first.Message.Thinking != "hmm" {
		t.Fatalf("thinking = %q", first.Message.Thinking)
	}
	if fn := requests[0]["tools"].([]any)[0].(map[string]any)["function"].(map[string]any); fn["parameters"] == nil {
		t.Fatalf("tool = %v", fn)
	}

	transcript := []Message{{Role: "user", Content: "show P1"}, first.Message, {Role: "tool", ToolCallID: "call_1", Content: "paper"}}
	second, err := provider.Complete(context.Background(), Request{Messages: transcript, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	if second.StopReason != "end_turn" || second.Message.Content != "Here it is." {
		t.Fatalf("second = %+v", second)
	}
	messages := requests[1]["messages"].([]any)
	assistant := messages[1].(map[string]any)
	if assistant["reasoning_content"] != "hmm" || assistant["tool_calls"] == nil {
		t.Fatalf("assistant turn lost raw fields: %v", assistant)
	}
	if tool := messages[2].(map[string]any); tool["role"] != "tool" || tool["tool_call_id"] != "call_1" || tool["content"] != "paper" {
		t.Fatalf("tool turn = %v", tool)
	}

	// A different protocol's raw turn is re-encoded, not replayed.
	foreign := Message{Role: "assistant", Content: "x", Raw: `[{"type":"text","text":"x"}]`, RawProtocol: ProtocolAnthropic}
	if _, err := provider.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "a"}, foreign, {Role: "user", Content: "b"}}}); err != nil {
		t.Fatal(err)
	}
	if turn := requests[2]["messages"].([]any)[1].(map[string]any); turn["content"] != "x" {
		t.Fatalf("foreign raw turn = %v", turn)
	}
}

func TestProviderErrorsCarryServiceMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	}))
	defer server.Close()
	provider, err := NewProvider(Config{Protocol: ProtocolAnthropic, BaseURL: server.URL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAndNormalize(t *testing.T) {
	normalized := Normalize(Config{})
	if normalized.Protocol != ProtocolAnthropic || normalized.BaseURL != DefaultAnthropicBaseURL || normalized.Model != DefaultAnthropicModel || normalized.MaxSteps != DefaultMaxSteps {
		t.Fatalf("defaults = %+v", normalized)
	}
	if err := Validate(Config{Protocol: "openai"}); err == nil {
		t.Fatal("openai without a model accepted")
	}
	if err := Validate(Config{Protocol: "gemini", Model: "x"}); err == nil {
		t.Fatal("unknown protocol accepted")
	}
	if err := Validate(Config{Protocol: "openai", BaseURL: "localhost:11434", Model: "x"}); err == nil {
		t.Fatal("base URL without scheme accepted")
	}
	if err := Validate(Config{Protocol: "openai", BaseURL: "http://localhost:11434/v1", Model: "qwen"}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Config{Model: "x", Effort: "turbo"}); err == nil {
		t.Fatal("unknown effort accepted")
	}
	if err := Validate(Config{Model: "x", Effort: " High "}); err != nil {
		t.Fatal(err)
	}
}

func TestMCPToolsOverLocalSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := library.Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	local, err := library.ConnectLocalMCP(ctx, store, func() library.SourceCredentials { return library.SourceCredentials{} })
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	tools := MCPTools{Session: local.Session}
	defs, err := tools.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Name] = true
		if def.InputSchema == nil {
			t.Fatalf("%s has no input schema", def.Name)
		}
	}
	for _, want := range []string{"describe_library", "list_papers", "add_paper", "add_evidence"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}
	// The in-process session writes without a bearer token.
	output, isError, err := tools.Call(ctx, "add_paper", json.RawMessage(`{"title":"Local agent paper","proposedScreening":"included"}`))
	if err != nil || isError {
		t.Fatalf("add_paper = %q, %v, %v", output, isError, err)
	}
	var paper library.Paper
	if err := json.Unmarshal([]byte(output), &paper); err != nil || paper.ID == "" || paper.Screening != "pending" {
		t.Fatalf("add_paper output %q: %v", output, err)
	}
	output, isError, err = tools.Call(ctx, "get_paper", json.RawMessage(`{"id":"missing"}`))
	if err != nil || !isError || output == "" {
		t.Fatalf("get_paper missing = %q, %v, %v", output, isError, err)
	}
	if _, isError, err := tools.Call(ctx, "no_such_tool", json.RawMessage(`{}`)); err != nil || !isError {
		t.Fatalf("unknown tool: isError=%v err=%v", isError, err)
	}
	if _, isError, _ := tools.Call(ctx, "list_papers", json.RawMessage(`[1]`)); !isError {
		t.Fatal("non-object arguments accepted")
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("字", MaxToolOutput)
	short := Truncate(long)
	if len(short) > MaxToolOutput+100 || !strings.HasSuffix(short, "bytes shown]") || !strings.HasPrefix(short, "字") {
		t.Fatalf("truncate = %d bytes, tail %q", len(short), short[len(short)-30:])
	}
	if Truncate("ok") != "ok" {
		t.Fatal("short text changed")
	}
}
