package tools

import (
	"context"
	"encoding/json"
)

type ToolCall struct {
    Name   string
    Args   map[string]interface{}
    Caller string
    Trace  []string // For tracking call stack or chains
}

type ToolResult struct {
    Output interface{}
    Error  error
}

type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]string // name:type
    Call(ctx context.Context, call ToolCall) ToolResult
}

// ParseToolCall tries to extract a tool call from LLM output.
func ParseToolCall(llmResp string) (ToolCall, bool) {
    // This function should parse JSON, function call, or even natural-language cues.
    // Example: look for a JSON block with "tool": "...", "args": {...}
    var toolCall ToolCall
    if err := json.Unmarshal([]byte(llmResp), &toolCall); err == nil && toolCall.Name != "" {
        return toolCall, true
    }



    // var suggestion ToolSuggestion
    // if json.Unmarshal([]byte(llmResp), &suggestion) == nil && suggestion.Tool != "" {
    //     toolCall := tools.ToolCall{
    //         Name:   suggestion.Tool,
    //         Args:   suggestion.Args,
    //         Caller: a.name,
    //         Trace:  []string{a.name},
    //     }


    return ToolCall{}, false
}