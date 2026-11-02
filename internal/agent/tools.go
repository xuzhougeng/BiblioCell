package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tools is what the loop can hand the model: a catalog and a way to run one.
type Tools interface {
	List(ctx context.Context) ([]ToolDef, error)
	// Call runs a tool and returns its output for the model; isError marks a
	// failure the model should see and recover from, not a loop failure.
	Call(ctx context.Context, name string, arguments json.RawMessage) (output string, isError bool, err error)
}

// MaxToolOutput bounds what one tool result adds to the context.
const MaxToolOutput = 60_000

// MCPTools exposes an MCP session's tools to the loop. The library's own MCP
// server, connected in-process, is the built-in tool set.
type MCPTools struct {
	Session *mcp.ClientSession
}

func (t MCPTools) List(ctx context.Context) ([]ToolDef, error) {
	var defs []ToolDef
	var cursor string
	for {
		result, err := t.Session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		for _, tool := range result.Tools {
			schema := tool.InputSchema
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			defs = append(defs, ToolDef{Name: tool.Name, Description: tool.Description, InputSchema: schema})
		}
		if result.NextCursor == "" {
			return defs, nil
		}
		cursor = result.NextCursor
	}
}

func (t MCPTools) Call(ctx context.Context, name string, arguments json.RawMessage) (string, bool, error) {
	var args map[string]any
	if err := json.Unmarshal(arguments, &args); err != nil {
		return "arguments must be a JSON object: " + err.Error(), true, nil
	}
	result, err := t.Session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		if ctx.Err() != nil {
			return "", false, err
		}
		// A protocol error (unknown tool, invalid params) is still something
		// the model can act on.
		return err.Error(), true, nil
	}
	return Truncate(toolText(result)), result.IsError, nil
}

// toolText prefers the structured result, which carries every field, over the
// text blocks that mirror it.
func toolText(result *mcp.CallToolResult) string {
	if result.StructuredContent != nil {
		if data, err := json.Marshal(result.StructuredContent); err == nil {
			return string(data)
		}
	}
	var parts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Truncate keeps a tool result within MaxToolOutput, saying so at the end.
func Truncate(text string) string {
	if len(text) <= MaxToolOutput {
		return text
	}
	cut := MaxToolOutput
	for cut > 0 && !isRuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + fmt.Sprintf("\n…[truncated: %d of %d bytes shown]", cut, len(text))
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
