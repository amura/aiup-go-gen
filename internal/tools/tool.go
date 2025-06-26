package tools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"aiupstart.com/go-gen/internal/common"
)


type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]interface{} // name:type
    Call(ctx context.Context, call common.ToolCall) common.ToolResult
    // Run(ctx context.Context, call *common.ToolCall) (*model.ToolResult, error)

}



// ParseToolCall tries to extract a tool call from LLM output.
func ParseToolCall(llmResp string) (common.ToolCall, bool) {
    // This function should parse JSON, function call, or even natural-language cues.
    // Example: look for a JSON block with "tool": "...", "args": {...}
    jsonBlock := ExtractFirstJsonBlock(llmResp)
    var toolCall common.ToolCall
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


    return common.ToolCall{}, false
}

func ParseToolCallToModel(content string) (*common.ToolCall, bool) {
	// First, try to find a JSON code block.
	re := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	matches := re.FindStringSubmatch(content)
	var jsonBlock string
	if len(matches) >= 2 {
		jsonBlock = matches[1]
	} else {
		// Fallback: just scan for any {...} with "tool": inside
		re2 := regexp.MustCompile(`(?s)\{[^}]*"tool"\s*:\s*"[^"]+"[^}]*\}`)
		matches2 := re2.FindStringSubmatch(content)
		if len(matches2) >= 1 {
			jsonBlock = matches2[0]
		}
	}

	if jsonBlock == "" {
		return nil, false
	}

	// Canonicalize (remove markdown etc.)
	jsonBlock = strings.TrimSpace(jsonBlock)

	// Unmarshal into map first for flexibility.
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonBlock), &m); err != nil {
		return nil, false
	}

	toolName, _ := m["tool"].(string)
	if toolName == "" {
		return nil, false
	}

	// Args: try to preserve all fields except "tool" and "caller" as arguments.
	args := make(map[string]interface{})
	for k, v := range m {
		if k != "tool" && k != "caller" {
			args[k] = v
		}
	}
	caller, _ := m["caller"].(string)
	return &common.ToolCall{
		Name:   toolName,
		Args:   args,
		Caller: caller,
	}, true
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

