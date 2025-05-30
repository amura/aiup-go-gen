// internal/model/message.go
package model

import "aiupstart.com/go-gen/internal/tools"

type MessageType string

const (
    TypeChat     MessageType = "chat"
    TypeToolCall MessageType = "tool_call"
    TypeToolResult MessageType = "tool_result"
    TypeRoute      MessageType = "route"
    TypeDirect     MessageType = "direct"
)

type Message struct {
    Sender  string
    Content string
    MessageType MessageType
    ToolCall    *tools.ToolCall // if tool_call
    ToolResult  *tools.ToolResult // if tool_result
    RouteTarget string // For routing messages to specific agents
}