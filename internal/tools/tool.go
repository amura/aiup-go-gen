package tools

import (
	"context"
	"encoding/json"
	"regexp"
)

type ToolCall struct {
    Name   string `json:"tool"`
    Args   map[string]interface{} `json:"args"` // Arguments for the tool, e.g. {"query": "search term"}
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
    jsonBlock := ExtractFirstJsonBlock(llmResp)
    var toolCall ToolCall
    if err := json.Unmarshal([]byte(jsonBlock), &toolCall); err == nil && toolCall.Name != "" {
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

// Returns first JSON code block if present, else empty string
func ExtractFirstJsonBlock(s string) string {
    // Regex for ```json ... ```
    re := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
    matches := re.FindStringSubmatch(s)
    if len(matches) >= 2 {
        return matches[1]
    }
    // Fallback: try to find any {...} JSON
    re2 := regexp.MustCompile("(?s)(\\{\\s*\"tool\"\\s*:\\s*\"[^\"]+\".*\\})")
    matches2 := re2.FindStringSubmatch(s)
    if len(matches2) >= 2 {
        return matches2[1]
    }
    return ""
}

