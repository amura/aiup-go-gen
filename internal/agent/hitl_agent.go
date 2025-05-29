// internal/agent/user_proxy.go
package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
)
type HITLAgent struct {
    name         string
    toolRegistry *tools.ToolRegistry // For listing/dispatching tools if user wants to call one manually
    ApproveTools bool // <----: Toggle approval
}

func NewHITLAgent(name string, registry *tools.ToolRegistry) *HITLAgent {
    return &HITLAgent{name: name, toolRegistry: registry}
}

func (h *HITLAgent) Name() string { return h.name }

// Listen for messages and prompt for tool execution if needed
func (h *HITLAgent) Start(input <-chan model.Message, output chan<- model.Message) {
    go func() {
        for msg := range input {
            if msg.MessageType == model.TypeToolCall && msg.ToolCall != nil {
                if h.ApproveTools {
                    fmt.Printf("\n[Assistant suggests tool: %s] Args: %v\n", msg.ToolCall.Name, msg.ToolCall.Args)
                    fmt.Print("Approve tool execution? (y/n/edit): ")
                    userInput := waitForUserInput()
                    switch userInput {
                    case "y", "Y":
                        // approved, execute tool
                        result := h.toolRegistry.CallTool(nil, *msg.ToolCall)
                        output <- model.Message{Sender: h.name, Content: fmt.Sprintf("%v", result.Output), MessageType: model.TypeToolResult, ToolResult: &result}
                    case "edit":
                        fmt.Print("Edit tool call JSON: ")
                        raw := waitForUserInput()
                        var editedCall tools.ToolCall
                        if err := json.Unmarshal([]byte(raw), &editedCall); err != nil {
                            fmt.Println("Invalid JSON, skipping tool call.")
                            output <- model.Message{Sender: h.name, Content: "[TOOL] Invalid JSON, skipped.", MessageType: model.TypeToolResult}
                            continue
                        }
                        result := h.toolRegistry.CallTool(nil, editedCall)
                        output <- model.Message{Sender: h.name, Content: fmt.Sprintf("%v", result.Output), MessageType: model.TypeToolResult, ToolResult: &result}
                    default:
                        fmt.Println("Tool execution skipped.")
                        output <- model.Message{Sender: h.name, Content: "[TOOL] Execution skipped by user.", MessageType: model.TypeToolResult}
                    }
                } else {
                    // Auto-approve: just execute the tool immediately
                    result := h.toolRegistry.CallTool(nil, *msg.ToolCall)
                    output <- model.Message{Sender: h.name, Content: fmt.Sprintf("%v", result.Output), MessageType: model.TypeToolResult, ToolResult: &result}
                }
            } else {
                fmt.Printf("\n[From %s]: %s\n", msg.Sender, msg.Content)
            }
        }
    }()
}

// Let user inject their own messages or tool calls
func (h *HITLAgent) UserInputLoop(output chan<- model.Message) {
    scanner := bufio.NewScanner(os.Stdin)
    for {
        fmt.Print("[You]: ")
        if !scanner.Scan() {
            break
        }
        userInput := scanner.Text()
        if userInput == "/tools" {
            fmt.Println("Available tools:")
            for _, t := range h.toolRegistry.List() {
                fmt.Printf("  %s: %s\n", t.Name(), t.Description())
            }
            continue
        }
        // Allow user to inject a tool call as JSON with /toolcall {json...}
        if len(userInput) > 9 && userInput[:9] == "/toolcall" {
            var call tools.ToolCall
            if err := json.Unmarshal([]byte(userInput[9:]), &call); err == nil {
                output <- model.Message{Sender: h.name, Content: string(userInput[9:])}
            } else {
                fmt.Println("Invalid tool call JSON.")
            }
            continue
        }
        output <- model.Message{Sender: h.name, Content: userInput}
    }
}

// Helper for blocking stdin prompt
func waitForUserInput() string {
    scanner := bufio.NewScanner(os.Stdin)
    if scanner.Scan() {
        return scanner.Text()
    }
    return ""
}
