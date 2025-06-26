package model

import (
	"encoding/json"

	"aiupstart.com/go-gen/internal/common"
)


const (
	RoleUser      string = "user"
	RoleAssistant string = "assistant"
	RoleTool      string = "tool"
	RoleSystem    string = "system"
	RoleManager   string = "manager"
	RoleOrch      string = "orchestrator"
	RoleUX        string = "ux"
    RoleOrchestrator = "orchestrator"
)

type MessageType string

const (
	TypeChat      MessageType = "chat"
	TypeToolCall  MessageType = "tool_call"
	TypeToolResult MessageType = "tool"
	TypeRoute     MessageType = "route"
	TypeError     MessageType = "error"
	TypeSystem    MessageType = "system"
	TypeDone	  MessageType = "done"
)

type AgentMessage struct {
	Role        string                   `json:"role"`
	Sender        string                   `json:"sender"`
	Type        MessageType            `json:"type"`
	Content     string                 `json:"content,omitempty"`
	ToolCall    *common.ToolCall              `json:"tool_call,omitempty"`
	ToolCallID  string                 `json:"call_id,omitempty"` // ID of the tool call, if applicable
	ToolResult  *ToolResult            `json:"tool_result,omitempty"`
	RouteTarget string                 `json:"route_target,omitempty"`
	OriginAgent string                 `json:"origin_agent,omitempty"`
	OriginContent string               `json:"origin_content,omitempty"`
	Tokens      *int                   `json:"tokens,omitempty"`
	ErrorDetail *ErrorDetail           `json:"error_detail,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
}

// type ToolCall struct {
// 	Name   string                 `json:"name"`
// 	Args   map[string]interface{} `json:"args"`
// 	Caller string                 `json:"caller,omitempty"`
// }

type ToolResult struct {
	Name      string      `json:"name"`
	Output    interface{} `json:"output,omitempty"`
	Error     string      `json:"error,omitempty"`
	ErrorMsg  string      `json:"error_msg,omitempty"`
	ErrorType string      `json:"error_type,omitempty"`
	ErrorPhase string      `json:"error_phase,omitempty"`
	Role      string        `json:"role,omitempty"`
	Phase     string      `json:"phase,omitempty"`
	Command   string      `json:"command,omitempty"`
	CmdOutput string      `json:"cmd_output,omitempty"`
}

type ErrorDetail struct {
	Phase   string `json:"phase,omitempty"`
	Command string `json:"command,omitempty"`
	Output  string `json:"output,omitempty"`
	ErrMsg  string `json:"err_msg,omitempty"`
}

func SafeJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

// Flatten history by removing nested Contexts to avoid recursive expansion
func FlattenHistory(history []AgentMessage) []AgentMessage {
    out := make([]AgentMessage, len(history))
    for i, msg := range history {
        msgCopy := msg
        msgCopy.Context = nil // REMOVE recursive context
        out[i] = msgCopy
    }
    return out
}

// NEW: Helper to build a flat history slice for LLMs, with no nested context/history fields.
func BuildChatHistory(ctx map[string]interface{}) []AgentMessage {
	if ctx == nil {
		return nil
	}
	raw, ok := ctx["history"]
	if !ok {
		return nil
	}
	rawHist, ok := raw.([]AgentMessage)
	if !ok {
		return nil
	}
	flat := make([]AgentMessage, 0, len(rawHist))
	for _, m := range rawHist {
		// copy only shallow values, drop context/history to prevent recursion
		copy := m
		copy.Context = nil // Drop context from each historical message
		flat = append(flat, copy)
	}
	return flat
}