package tools

import (
    "context"
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