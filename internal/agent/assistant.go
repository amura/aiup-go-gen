// internal/agent/assistant.go
package agent

import (
	"context"
	"fmt"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"

	"encoding/json"
)

type AssistantAgent struct {
    name         string
    llmClient    llm.LLMClient
    promptTpl    string
    toolRegistry *tools.ToolRegistry
}

func NewAssistantAgent(name string, llmClient llm.LLMClient, promptTpl string, registry *tools.ToolRegistry) *AssistantAgent {
    return &AssistantAgent{
        name:         name,
        llmClient:    llmClient,
        promptTpl:    promptTpl,
        toolRegistry: registry,
    }
}

type ToolSuggestion struct {
    Tool string                 `json:"tool"`
    Args map[string]interface{} `json:"args"`
}

func (a *AssistantAgent) Name() string { return a.name }

func (a *AssistantAgent) Start(input <-chan model.Message, output chan<- model.Message) {
    go func() {
        for msg := range input {
            prompt := fmt.Sprintf(a.promptTpl, msg.Content)
			utils.Logger.Debug().Str("prompt", prompt).Msg("Prompt sent to LLM")
            llmResp, err := a.llmClient.Generate(prompt)
			utils.Logger.Debug().Str("llm_response", llmResp).Msg("LLM response received")
            if err != nil {
                output <- model.Message{Sender: a.name, Content: "[ERROR] " + err.Error()}
                continue
            }
			fmt.Printf("[LLM Response from %s]: %s\n", a.name, llmResp)
			utils.Logger.Debug().Str(("llm_response"), llmResp).Msg("About to parse response and check for tool calling")
            // Try parsing as a tool suggestion
            var suggestion ToolSuggestion
            if json.Unmarshal([]byte(llmResp), &suggestion) == nil && suggestion.Tool != "" {
                toolCall := tools.ToolCall{
                    Name:   suggestion.Tool,
                    Args:   suggestion.Args,
                    Caller: a.name,
                    Trace:  []string{a.name},
                }
                utils.Logger.Debug().Str("tool_call", fmt.Sprintf("%+v", toolCall)).Msg("Tool call created from LLM response")
                result := a.toolRegistry.CallTool(context.Background(), toolCall)
                if result.Error != nil {
                    utils.Logger.Error().Err(result.Error).Msg("Tool call failed")
                    output <- model.Message{Sender: a.name, Content: "[TOOL ERROR] " + result.Error.Error()}
                } else {
                    utils.Logger.Debug().Str("tool_result", fmt.Sprintf("%+v", result.Output)).Msg("Tool call succeeded")
                    output <- model.Message{Sender: a.name, Content: fmt.Sprintf("%v", result.Output)}
                }
            } else {
                utils.Logger.Debug().Msg("No tool call detected in LLM response, sending direct response")
                // Otherwise, just output the LLM’s direct response
                output <- model.Message{Sender: a.name, Content: llmResp}
            }
        }
    }()
}