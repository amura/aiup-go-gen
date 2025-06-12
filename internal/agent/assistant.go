// internal/agent/assistant.go
package agent

import (
	"fmt"
	"regexp"
	"strings"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"
)

const assistantPromptTemplate = `
You are an expert AI coding assistant. Your persona: %s

When the user requests code execution or bug fixing, use the docker_exec tool.
For multi-file outputs, provide code_blocks as an array of objects.
Each object should have:
- language: file language (e.g. python, bash)
- filename: e.g. main.py
- code: the code/content as a string

You must pass in the dockerfile content inside the docker_file parameter which can be used to setup an image that will have all the required dependencies installed and configured

Do not output code as plain strings or markdown—always use this structure for tool calls.

Do not emit code, tool calls, or JSON directly in your message content. Only use tool calls for execution.

When generating shell or CLI commands, you must always include flags that ensure NO user interaction or prompts (for example, use "--no-interactive" and "--defaults" for Angular CLI commands). Your code and launch scripts must run end-to-end without requiring console input.

Otherwise, reply with your answer directly.

---------------------------------------------

You have access to the following tools:
%s

---------------------------------------------

If the previous execution failed, analyze the error shown, fix the code and retry.

------------------------------------

User request: %s
`

type AssistantAgent struct {
	name         string
	llmClient    llm.LLMClient
	persona      string
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

func (a *AssistantAgent) Name() string { return a.name }

func (a *AssistantAgent) Start(input <-chan model.Message, output chan<- model.Message) {
	go func() {
		for msg := range input {
			metrics.AgentMessagesTotal.WithLabelValues(a.name).Inc()
			// Compose a prompt that includes both persona and available tools
			// prompt := a.promptTpl + "\n" + a.toolsPrompt + "\nUser: " + msg.Content
			prompt := fmt.Sprintf(
				strings.ReplaceAll(assistantPromptTemplate, "T_B_T", "```"),
				a.persona,
				a.toolRegistry.DescribeTools(),
				msg.Content,
			)

			utils.Logger.Debug().Str("prompt", prompt[:100]).Msg("Prompt going to LLM")
			llmResp, err := a.llmClient.Generate(prompt)
			if err != nil {
				output <- model.Message{Sender: a.name, Content: "[LLM ERROR] " + err.Error()}
				continue
			}

			// utils.Logger.Debug().Str("llm_response", llmResp.Content).Msg("LLM response received")
			// if err != nil {
			//     output <- model.Message{Sender: a.name, Content: "[LLM ERROR] " + err.Error()}
			//     continue
			// }
			// fmt.Printf("[LLM Response from %s]: %s\n", a.name, llmResp)
			// utils.Logger.Debug().Str(("llm_response"), llmResp.Content).Msg("About to parse response and check for tool calling")

			// // --- OpenAI function calling: check ToolCalls ---
			// if len(llmResp.ToolCalls) > 0 {
			//     for _, toolCall := range llmResp.ToolCalls {
			//         if a.toolRegistry.HasTool(toolCall.Name) {
			//             utils.Logger.Debug().
			//                 Str("tool_call", fmt.Sprintf("%+v", toolCall)).
			//                 Msg("Tool call from OpenAI response")
			//             output <- model.Message{
			//                 Sender:      a.name,
			//                 MessageType: model.TypeToolCall,
			//                 ToolCall: &tools.ToolCall{
			//                     Name:   toolCall.Name,
			//                     Args:   toolCall.Args,
			//                     Caller: a.name,
			//                 },
			//                 Content: fmt.Sprintf("Tool call for %s", toolCall.Name),
			//             }
			//         }
			//     }
			//     continue
			// }



            // --- OpenAI function calling: check ToolCalls ---
            if len(llmResp.ToolCalls) > 0 {
                for _, toolCall := range llmResp.ToolCalls {
                    if a.toolRegistry.HasTool(toolCall.Name) {
                        utils.Logger.Debug().
                            Str("tool_call", fmt.Sprintf("%+v", toolCall.Name)).
                            Msg("Tool call from OpenAI response")
                        output <- model.Message{
                            Sender:      a.name,
                            MessageType: model.TypeToolCall,
                            ToolCall: &tools.ToolCall{
			                    Name:   toolCall.Name,
			                    Args:   toolCall.Args,
			                    Caller: a.name,
			                },
                        }
                    }
                }
                continue
            }

			// Try parsing as a tool suggestion
			toolCall, toolDetected := tools.ParseToolCall(llmResp.Content)
			if toolDetected && a.toolRegistry.HasTool(toolCall.Name) {
				utils.Logger.Debug().Str("tool_call", fmt.Sprintf("%+v", toolCall)).Msg("Tool call created from LLM response\n")

				// Instead of running, delegate to HITL agent by sending tool call message
				// output <- model.Message{Sender: a.name, Content: llmResp.Content, MessageType: model.TypeToolCall, ToolCall: &toolCall}
				output <- model.Message{
					Sender:      a.name,
					MessageType: model.TypeToolCall,
					ToolCall:    &toolCall,
				}
				continue

			}
			utils.Logger.Debug().Msg("No tool call detected in LLM response, sending direct response")
			// Otherwise, just output the LLM’s direct response
			// Otherwise, just output the LLM’s direct response
			output <- model.Message{
				Sender:      a.name,
				Content:     llmResp.Content,
				MessageType: model.TypeChat,
			}
		}
	}()
}

// Returns first JSON code block if present, else empty string
func ExtractFirstJsonBlock(s string) string {
	// Regex for ```json ... ```
	re := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	matches := re.FindStringSubmatch(s)
	if len(matches) >= 2 {
		return matches[1]
	}
	// Fallback: try to find any {...} JSON
	re2 := regexp.MustCompile("(?s)(\\{\\s*\"tool\"\\s*:\\s*\"[^\"]+\".*\\})")
	matches2 := re2.FindStringSubmatch(s)
	if len(matches2) >= 2 {
		return matches2[1]
	}
	return ""
}

