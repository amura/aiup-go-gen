// internal/agent/assistant.go
package agent

import (
	"fmt"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"

)

const promptTemplate = `You are an AI assistant. Your persona is: %s

You have access to the following tools:
%s

When appropriate, call a tool by outputting JSON like:
{"tool": "fetch_arxiv", "args": {"query": "..."}}.

Otherwise, answer directly.

User request: %s
`

type AssistantAgent struct {
    name         string
    llmClient    llm.LLMClient
    persona    string
    toolRegistry *tools.ToolRegistry
}

func NewAssistantAgent(name string, llmClient llm.LLMClient, persona string, registry *tools.ToolRegistry) *AssistantAgent {
    return &AssistantAgent{
        name:         name,
        llmClient:    llmClient,
        toolRegistry: registry,
        persona:      persona,
        // toolsPrompt:  registry.DescribeTools(),
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
            // Compose a prompt that includes both persona and available tools
            // prompt := a.promptTpl + "\n" + a.toolsPrompt + "\nUser: " + msg.Content
            prompt := fmt.Sprintf(
                promptTemplate,
                a.persona,
                a.toolRegistry.DescribeTools(),
                msg.Content,
            )

			utils.Logger.Debug().Str("prompt", prompt).Msg("Prompt going to LLM")
            llmResp, err := a.llmClient.Generate(prompt)
            // prompt := fmt.Sprintf(a.promptTpl, msg.Content)


			utils.Logger.Debug().Str("llm_response", llmResp).Msg("LLM response received")
            if err != nil {
                output <- model.Message{Sender: a.name, Content: "[ERROR] " + err.Error()}
                continue
            }
			fmt.Printf("[LLM Response from %s]: %s\n", a.name, llmResp)
			utils.Logger.Debug().Str(("llm_response"), llmResp).Msg("About to parse response and check for tool calling")

            // Try parsing as a tool suggestion
            toolCall, toolDetected := tools.ParseToolCall(llmResp)
            if toolDetected && a.toolRegistry.HasTool(toolCall.Name) {
                utils.Logger.Debug().Str("tool_call", fmt.Sprintf("%+v", toolCall)).Msg("Tool call created from LLM response")
                // result := a.toolRegistry.CallTool(context.Background(), toolCall)
                // if result.Error != nil {
                //     utils.Logger.Error().Err(result.Error).Msg("Tool call failed")
                //     output <- model.Message{Sender: a.name, Content: "[TOOL ERROR] " + result.Error.Error()}
                // } else {
                //     utils.Logger.Debug().Str("tool_result", fmt.Sprintf("%+v", result.Output)).Msg("Tool call succeeded")
                //     output <- model.Message{Sender: a.name, Content: fmt.Sprintf("%v", result.Output)}
                // }
                  // Instead of running, delegate to HITL agent by sending tool call message
                  output <- model.Message{Sender: a.name, Content: llmResp, MessageType: model.TypeToolCall, ToolCall: &toolCall}
           
            } else {
                utils.Logger.Debug().Msg("No tool call detected in LLM response, sending direct response")
                // Otherwise, just output the LLM’s direct response
                output <- model.Message{Sender: a.name, Content: llmResp, MessageType: model.TypeChat}
            }
        }
    }()
}

