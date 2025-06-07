// internal/model/message.go
package model

import ( 
    "aiupstart.com/go-gen/internal/tools"
    openai "github.com/sashabaranov/go-openai"
)


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
    IsError bool // Indicates if this message is an error
    Error error
    OriginAgent    string // Who initiated this request
    OriginContent  string // What was the original subtask/request
    Tokens *openai.Usage // For LLM responses, if applicable
}