package agent

import (
	"context"
	"fmt"

	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"
)

type ToolRunnerAgent struct {
	name     string
	registry *tools.ToolRegistry
}

func NewToolRunnerAgent(name string, registry *tools.ToolRegistry) *ToolRunnerAgent {
	return &ToolRunnerAgent{name: name, registry: registry}
}
func (a *ToolRunnerAgent) Name() string { return a.name }
func (a *ToolRunnerAgent) Start(input <-chan model.Message, output chan<- model.Message) {
	go func() {
		for msg := range input {
			fmt.Println("ToolRunner received message!")
			utils.Logger.Debug().
				Str("agent", a.name).
				Str("event", "received_message").
				Msgf("Received: %s", msg.Content)
			if msg.MessageType == model.TypeToolCall && msg.ToolCall != nil {
				result := a.registry.Call(context.TODO(), *msg.ToolCall)
				output <- model.Message{
					Sender:      a.name,
					Content:     fmt.Sprintf("%v", result.Output), // Safely stringify any output,
					MessageType: model.TypeToolResult,
				}
			} else {
				utils.Logger.Warn().
					Str("agent", a.name).
					Msgf("Received non-tool call message: %s", msg.Content)
			
			}
		}
	}()
}