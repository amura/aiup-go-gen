package agent

import (
	"context"
	"fmt"

	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"
)

type ToolRunnerAgent struct {
	name           string
	description    string
	registry       *tools.ToolRegistry
	dockerExecTool *tools.DockerExecTool
}

func NewToolRunnerAgent(name string, registry *tools.ToolRegistry) *ToolRunnerAgent {
	return &ToolRunnerAgent{name: name, registry: registry}
}
func (a *ToolRunnerAgent) Name() string        { return a.name }
func (a *ToolRunnerAgent) Description() string { return a.description }
func (a *ToolRunnerAgent) SetDescription(desc string) {
	a.description = desc
}
func (a *ToolRunnerAgent) CleanupContainer(ctx context.Context) error {
	if a.dockerExecTool != nil {
		utils.Logger.Info().Str("agent", a.name).Msg("Cleaning up DockerExecTool container")
		return a.dockerExecTool.CleanupContainer(ctx)
	}
	utils.Logger.Warn().Str("agent", a.name).Msg("No DockerExecTool to cleanup")
	return nil
}

func (a *ToolRunnerAgent) Start(input <-chan model.AgentMessage, output chan<- model.AgentMessage) {
	go func() {
		for msg := range input {
			utils.Logger.Debug().
				Str("agent", a.name).
				Str("event", "received_message").
				Msgf("Received: %s", msg.Content)
			utils.LogContext(msg.Context, "ToolRunner received context")

			ctx := msg.Context
			if ctx == nil {
				ctx = map[string]interface{}{}
			}

			if msg.Type == model.TypeToolCall && msg.ToolCall != nil {
				result := a.registry.Call(context.TODO(), *msg.ToolCall)

				utils.Logger.Debug().
					Str("agent", a.name).
					Str("tool", msg.ToolCall.Name).
					Msgf("Tool call result:\n\n  %v \n\n", result.Output)

				// On error, add error/output to context for Assistant
				if result.Error != nil {
					ctx["tool_error"] = result.Output
					ctx["tool_error_detail"] = result.Error.Error()
					ctx["tool_phase"] = "tool_call"
					ctx["tool_name"] = msg.ToolCall.Name
					ctx["tool_id"] = msg.ToolCall.ID
				}

				// Always route result back to the original caller (assistant, etc)
				var errorMsg string
				if result.Error != nil {
					errorMsg = result.Error.Error()
				}

				output <- model.AgentMessage{
					Role:        "tool",
					Sender:      a.name,
					Content:     fmt.Sprintf("%v", result.Output),
					Type:        model.TypeToolResult,
					RouteTarget: msg.ToolCall.Caller, // Dynamic! E.g. "assistant"
					ErrorDetail: &model.ErrorDetail{
						Phase:   "tool_call",
						Command: msg.ToolCall.Name,
						Output:  fmt.Sprintf("%v", result.Output),
						ErrMsg:  fmt.Sprintf("%v", result.Error),
					},
					OriginAgent:   msg.Sender,
					OriginContent: msg.Content,
					Context:       ctx,
					ToolCall:      msg.ToolCall,
					ToolCallID:    msg.ToolCall.ID,
					ToolResult:    &model.ToolResult{Output: result.Output, Error: errorMsg},
				}
			} else {
				utils.Logger.Warn().
					Str("agent", a.name).
					Msgf("Received non-tool call message: %s", msg.Content)
			}
		}
	}()
}
