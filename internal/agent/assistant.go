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
	name           string
	description    string
	llmClient      llm.LLMClient
	persona        string
	toolRegistry   *tools.ToolRegistry
	promptOverload string
}

func NewAssistantAgent(name string, llmClient llm.LLMClient, persona string, promptOverload string, registry *tools.ToolRegistry) *AssistantAgent {
	return &AssistantAgent{
		name:           name,
		llmClient:      llmClient,
		toolRegistry:   registry,
		persona:        persona,
		promptOverload: promptOverload,
	}
}

func (a *AssistantAgent) Name() string        { return a.name }
func (a *AssistantAgent) Description() string { return a.description }
func (a *AssistantAgent) SetDescription(desc string) { a.description = desc }

func (a *AssistantAgent) Start(input <-chan model.Message, output chan<- model.Message) {
	go func() {
		for msg := range input {
			metrics.AgentMessagesTotal.WithLabelValues(a.name).Inc()

			// -------- Prompt Assembly --------
			ctx := msg.Context
			if ctx == nil { ctx = map[string]interface{}{} }
			contextSummary := buildContextSummary(ctx)
			var prompt string
			if a.promptOverload != "" {
				prompt = a.promptOverload
				if contextSummary != "" {
					prompt += "\n\n" + contextSummary
				}
				prompt += "\n\nUser request: " + msg.Content
			} else {
				prompt = fmt.Sprintf(
					strings.ReplaceAll(assistantPromptTemplate, "T_B_T", "```"),
					a.persona,
					a.toolRegistry.DescribeTools(),
					msg.Content,
				)
				if contextSummary != "" {
					// Insert context summary before user request if any
					parts := strings.Split(prompt, "User request:")
					if len(parts) == 2 {
						prompt = parts[0] + contextSummary + "\n\nUser request:" + parts[1]
					} else {
						prompt += "\n" + contextSummary
					}
				}
			}
			utils.Logger.Debug().Str("prompt", prompt[:100]).Msg("Prompt going to LLM")
			utils.LogContext(ctx, "Assistant input context") // [ADDED LOGGING]

			llmResp, err := a.llmClient.Generate(prompt)
			utils.Logger.Debug().Str("llm_response", fmt.Sprintf("%v", llmResp)).Msg("LLM response received")
			if err != nil {
				output <- model.Message{Sender: a.name, Content: "[LLM ERROR] " + err.Error(), Context: ctx}
				continue
			}

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
							Context:     ctx, // [ALWAYS PASS CONTEXT]
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

			// --- Try parsing as a tool suggestion ---
			toolCall, toolDetected := tools.ParseToolCall(llmResp.Content)
			if toolDetected && a.toolRegistry.HasTool(toolCall.Name) {
				utils.Logger.Debug().Str("tool_call", fmt.Sprintf("%+v", toolCall)).Msg("Tool call created from LLM response\n")
				output <- model.Message{
					Sender:      a.name,
					MessageType: model.TypeToolCall,
					ToolCall:    &toolCall,
					Context:     ctx,
				}
				continue
			}

			utils.Logger.Debug().Msg("No tool call detected in LLM response, sending direct response")
			output <- model.Message{
				Sender:      a.name,
				Content:     llmResp.Content,
				MessageType: model.TypeChat,
				Context:     ctx, // [ALWAYS PASS CONTEXT]
			}
		}
	}()
}

// Returns first JSON code block if present, else empty string
func ExtractFirstJsonBlock(s string) string {
	re := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	matches := re.FindStringSubmatch(s)
	if len(matches) >= 2 {
		return matches[1]
	}
	re2 := regexp.MustCompile("(?s)(\\{\\s*\"tool\"\\s*:\\s*\"[^\"]+\".*\\})")
	matches2 := re2.FindStringSubmatch(s)
	if len(matches2) >= 2 {
		return matches2[1]
	}
	return ""
}

// [ADDED]: Build generic context summary string for prompt
func buildContextSummary(ctx map[string]interface{}) string {
	if ctx == nil || len(ctx) == 0 { return "" }
	lines := []string{"# Context Summary:"}
	for k, v := range ctx {
		switch vv := v.(type) {
		case []string:
			lines = append(lines, fmt.Sprintf("- %s:\n%s", k, strings.Join(vv, "\n")))
		default:
			lines = append(lines, fmt.Sprintf("- %s: %v", k, v))
		}
	}
	return strings.Join(lines, "\n")
}

