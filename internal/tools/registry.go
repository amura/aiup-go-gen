package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type ToolRegistry struct {
    tools map[string]Tool
    mu    sync.RWMutex
}

func NewToolRegistry() *ToolRegistry {
    return &ToolRegistry{
        tools: make(map[string]Tool),
    }
}

func (r *ToolRegistry) Register(tool Tool) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.tools[tool.Name()] = tool
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    tool, ok := r.tools[name]
    return tool, ok
}

func (r *ToolRegistry) List() []Tool {
    r.mu.RLock()
    defer r.mu.RUnlock()
    out := make([]Tool, 0, len(r.tools))
    for _, t := range r.tools {
        out = append(out, t)
    }
    return out
}

// Generates a string like: "Available tools: fetch_arxiv: search academic papers; milvus_vector: vector DB ops; ..."
func (tr *ToolRegistry) DescribeTools() string {
    descs := []string{}
    for _, t := range tr.List() {
        descs = append(descs, fmt.Sprintf("%s: %s", t.Name(), t.Description()))
    }
    return "Available tools:\n" + strings.Join(descs, "\n")
}

// Dynamic tool invocation by name (with trace support)
func (r *ToolRegistry) CallTool(ctx context.Context, call ToolCall) ToolResult {
    tool, ok := r.Get(call.Name)
    if !ok {
        return ToolResult{Error: fmt.Errorf("tool not found: %s", call.Name)}
    }
    // Extend trace
    call.Trace = append(call.Trace, call.Name)
    return tool.Call(ctx, call)
}

func (r *ToolRegistry) HasTool(name string) bool {
    r.mu.RLock()
    defer r.mu.RUnlock()
    _, ok := r.tools[name]
    return ok
}