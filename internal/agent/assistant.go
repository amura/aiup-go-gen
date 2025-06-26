package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/google/uuid"
)

const defaultPrompt = `
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
If the previous execution failed, analyze the error shown, fix the code and retry.

`

const defaultPersona = `
You are an AI coding assistant with expertise in software development, debugging, and tool integration.
`

const outputInstructions = `

* IMPORTANT: If you produce a reply, you MUST reply only in strict JSON. Output nothing else, nor do not include any leading text or comments. Just return valid JSON objects:
N.B Do not use any backticks or T_B_T as this will be marshalled as a string in the JSON using 'Go', so string content must be compatible.

Format your output like this json object:
{
	"role": "ux",
	"type": "wireframe",
	"content": "ASCII wireframe and explanation here, without any T_B_T", 
	"route_target": "assistant"
}
	
Ensure this output is valid JSON:
- You must escape all newlines embedded in the content as \n 
- Replace or encode any strange characters such as backticks, so that it can be easily parsed in Go.
- Do not use any other format or free text, only valid JSON objects.


*	Use type: "tool_call" and set "route_target": "toolrunner" when code execution is needed.
*	Only return with no route_target if the process is fully done.

`

// persona, prompt, tools, user request

// AssistantAgent is a single-agent implementation using strict role/content JSON messaging.
type AssistantAgent struct {
	name            string
	role            string
	description     string
	llmClient       llm.LLMClient
	persona         string
	toolRegistry    *tools.ToolRegistry
	promptOverload  string // If present, overloads the default prompt template.
	toolAgentPrompt map[string]interface{}
}

func NewAssistantAgent(name string, role string, llmClient llm.LLMClient, persona string, promptOverload string, registry *tools.ToolRegistry) *AssistantAgent {
	return &AssistantAgent{
		name:            name,
		role:            role,
		llmClient:       llmClient,
		toolRegistry:    registry,
		persona:         persona,
		promptOverload:  promptOverload,
		toolAgentPrompt: make(map[string]interface{}), // Initialize as nil, will be set when tool calls are made
	}
}

func (a *AssistantAgent) Name() string               { return a.name }
func (a *AssistantAgent) Description() string        { return a.description }
func (a *AssistantAgent) SetDescription(desc string) { a.description = desc }

// BuildChatHistory transforms []AgentMessage into []llm.ChatMessage for LLM context.
func BuildChatHistory(history []model.AgentMessage) []model.AgentMessage {
	var out []model.AgentMessage
	for _, msg := range history {
		out = append(out, model.AgentMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCall:   msg.ToolCall,
			ToolResult: msg.ToolResult,
		})
	}
	return out
}

func (a *AssistantAgent) Start(input <-chan model.AgentMessage, output chan<- model.AgentMessage) {
	go func() {
		for msg := range input {
			utils.Logger.Debug().
				Str("agent", a.name).
				Str("event", "received_message").
				Msgf("Received message from : %s", msg.Sender)

			metrics.AgentMessagesTotal.WithLabelValues(a.name).Inc()
			// utils.LogContext(msg.Context, "Assistant input context")
			// Log only context keys, not full value, to avoid recursive dump
			if msg.Context != nil {
				keys := make([]string, 0, len(msg.Context))
				for k := range msg.Context {
					keys = append(keys, k)
				}
				utils.Logger.Debug().Strs("ctx_keys", keys).Msg("Assistant input context keys")
			}

			prompt := ""
			history := []model.AgentMessage{}

			if msg.ErrorDetail != nil && msg.ErrorDetail.Phase == "tool_call" {
				// If this is a tool call error, we need to handle it
				utils.Logger.Error().
					Str("agent", a.name).
					Msgf("Tool call error: %s", msg.ErrorDetail.ErrMsg)

				// get previous prompt and append the error details
				if toolID, ok := msg.Context["tool_id"].(string); ok && toolID != "" {

					if prevPrompt, ok := a.toolAgentPrompt[toolID]; ok {

						prompt = injectError(msg, prevPrompt.(string))
						history = model.BuildChatHistory(msg.Context)

						// // set the toolresult on the last history item
						// // if len(history) > 0 {
						// // 	lastIndex := len(history) - 1
						// // 	history[lastIndex].ToolResult = msg.ToolResult
						// // }

						// history = append(history, model.AgentMessage{
						// 	Role:       a.role,
						// 	Type:       model.TypeToolResult,
						// 	Content:    msg.Content,
						// 	ToolCall:   msg.ToolCall,
						// 	ToolResult: msg.ToolResult,
						// 	Context:    msg.Context,
						// })
					}
				}
			} else {

				// If context includes history, use it for LLM (Autogen style)
				// var history []model.AgentMessage
				// if hist, ok := msg.Context["history"].([]model.AgentMessage); ok && len(hist) > 0 {
				// 	history = model.FlattenHistory(hist)
				// } else {
				// 	history = model.FlattenHistory([]model.AgentMessage{msg})
				// }
				history = model.BuildChatHistory(msg.Context)

				// prompt := strings.ReplaceAll(a.promptOverload, "T_B_T", "```")
				// persona := a.persona
				// if persona == "" {
				// 	persona = defaultPersona
				// }

				// // if not prompt overload, use defaults
				// if prompt == "" {
				// 	utils.Logger.Debug().Msg("Using default prompt template for AssistantAgent")
				// 	// persona, prompt, tools, user request: formats
				// 	prompt = fmt.Sprintf(
				// 		strings.ReplaceAll(outputInstructions, "T_B_T", "```"),
				// 		persona,
				// 		defaultPrompt,
				// 		a.toolRegistry.DescribeTools(), // tools
				// 		msg.Content,  // user request
				// 	)
				// } else {
				// 	utils.Logger.Debug().Msg("Using prompt overload for AssistantAgent")
				// 	prompt = fmt.Sprintf(
				// 		strings.ReplaceAll(outputInstructions, "T_B_T", "```"),
				// 		persona,
				// 		prompt,
				// 		a.toolRegistry.DescribeTools(), // tools
				// 		msg.Content,  // user request
				// 	)
				// }

				prompt = BuildUpPrompt(history, msg.Content, a)

				// Build LLM prompt, injecting error details from previous tool execution if present.
				prompt = injectError(msg, prompt)

				// In your AssistantAgent.Start (pseudo code inside the tool error handling branch):
				// if LastToolFailed(history) {
				// 	prevAssistant := FindPrecedingAssistant(history)
				// 	if prevAssistant != nil {
				// 		prompt += "\n\n----\nPREVIOUS ASSISTANT OUTPUT (for repair):\n" + prevAssistant.Content + "\n----\n"
				// 	}
				// }

				// If the last tool call failed, inject previous assistant output for context
				if errText, ok := msg.Context["tool_error"].(string); ok && errText != "" {

					var uxSpec string
					if lastRelevant := FindLastRelevantContext(history, a.name); lastRelevant != nil {
						uxSpec = lastRelevant.Content
					}

					// Now build your prompt as before:
					if uxSpec != "" {
						prompt += "\n\n# Previous request context:\n" + uxSpec
					}
				}

				utils.Logger.Debug().
					Str("agent", a.name).
					Msgf("About to call llm with prompt %s", prompt[:min(50, len(prompt))]) // Log first 100 chars
			}

			filteredHistory := filterAgentHistory(history)
			// Call LLM with structured history, not just a string!
			llmResp, err := a.llmClient.Generate(filteredHistory, prompt)
			if err != nil {
				utils.Logger.Error().
					Err(err).
					Str("agent", a.name).
					Msg("LLM generation error")
				output <- model.AgentMessage{
					Role:    a.role,
					Type:    model.TypeError,
					Content: "[LLM ERROR] " + err.Error(),
					Context: filterContextForNext(msg.Context), // Pass only relevant context
				}
				continue
			}
			utils.Logger.Debug().Str("llm_response", fmt.Sprintf("%v", len(llmResp.Content))).Msg("LLM response received!!")

			// Handle tool calls if present.
			if len(llmResp.ToolCalls) > 0 {
				utils.Logger.Debug().Int("tool_calls_count", len(llmResp.ToolCalls)).Msg("LLM response contains tool calls")
				// Add the current prompt, the current agent message, and tool name to a dictionary

				for _, tc := range llmResp.ToolCalls {
					if tc.ID == "" {
						tc.ID = uuid.New().String() // Or whatever you use for unique IDs
					}
					am := model.AgentMessage{
						Role: a.role,
						Type: model.TypeToolCall,
						ToolCall: &common.ToolCall{
							Name:   tc.Name,
							Args:   tc.Args,
							Caller: a.name,
							ID:     tc.ID, // Use the LLM's tool call ID
						},
						Context:     msg.Context,
						RouteTarget: "toolrunner", // todo improve via e.g. a.toolRegistry.GetRunnerAgentName(tc.Name) or let orch decide by setting blank
					}
					a.toolAgentPrompt[tc.ID] = prompt // Use the agent name as the caller
					output <- am
				}
				continue
			}
			utils.Logger.Debug().Msg("LLM response does not contain tool calls")
			var replyMsg model.AgentMessage
			fixed := FixContentFieldNewlines(llmResp.Content)
			utils.Logger.Debug().
				Str("agent", a.name).Msg("About to parse LLM response content as AgentMessage")
			if err := json.Unmarshal([]byte(fixed), &replyMsg); err != nil {
				// handle error
				utils.Logger.Error().Err(err).Msgf("Failed to parse LLM response as AgentMessage \nContent: %s", llmResp.Content)
			}

			utils.Logger.Debug().
				Str("agent", a.name).
				Msg(replyMsg.Content)

			utils.Logger.Debug().
				Str("agent", a.name).
				Str("event", "llm_response").
				Str("role", string(replyMsg.Role)).
				Str("type", string(replyMsg.Type)).
				Msg("LLM response processed")

			output <- model.AgentMessage{
				Role:        a.role,
				Type:        model.TypeChat,
				Content:     replyMsg.Content,
				RouteTarget: replyMsg.RouteTarget,
				Context:     msg.Context,
				Tokens:      nil,
			}
		}
	}()
}

// Only add relevant roles to LLM history
func filterAgentHistory(history []model.AgentMessage) []model.AgentMessage {
	seenToolCallIDs := make(map[string]bool)
	out := []model.AgentMessage{}
	for _, m := range history {
		switch m.Role {

		case "orchestrator":
			// skip
		default:
			// Only deduplicate tool calls, let all other messages pass through
			if m.ToolCall != nil  {
				if m.ToolCall.ID == "" {
					utils.Logger.Error().Msg("Tool call message found in history with empty ID! This is a bug in the code that generates tool calls.")
                	panic("tool_call message found in history with empty ID! You must fix the code that generates tool calls.")
            	}
				if seenToolCallIDs[fmt.Sprintf("%s:%s", m.Role, m.ToolCall.ID)] {
					continue // skip duplicate
				}
				seenToolCallIDs[fmt.Sprintf("%s:%s", m.Role, m.ToolCall.ID)] = true
			}
			out = append(out, m)
			// Add more agent roles as you add them
		}
		// skip orchestrator, router, etc.
	}
	return out
}

func BuildUpPrompt(history []model.AgentMessage, userRequest string, a *AssistantAgent) string {

	persona := a.persona
	promptOverload := a.promptOverload
	if persona == "" {
		persona = defaultPersona
	}

	builder := strings.Builder{}

	builder.WriteString(persona)

	if promptOverload == "" {
		promptOverload = defaultPrompt
	}

	prompt := strings.ReplaceAll(promptOverload, "T_B_T", "```")

	builder.WriteString("\n\n")
	builder.WriteString(prompt)
	builder.WriteString("\n\n")

	if a.toolRegistry != nil {
		utils.Logger.Debug().Msg("Using tool registry")
		builder.WriteString("You have access to the following tools:\n")
		builder.WriteString(a.toolRegistry.DescribeTools())
		builder.WriteString("\n\n")
	}

	builder.WriteString("User request: ")
	builder.WriteString(userRequest)
	builder.WriteString("\n\n---------------------------\n\n")
	builder.WriteString(strings.ReplaceAll(outputInstructions, "T_B_T", "```"))
	builder.WriteString("\n\n")

	return builder.String()
}

func LastToolFailed(history []model.AgentMessage) bool {
	if len(history) == 0 {
		return false
	}
	last := history[len(history)-1]
	// Adjust this depending on how you mark errors
	return last.Type == "tool_result" && last.ErrorDetail != nil && last.ErrorDetail.ErrMsg != ""
}

func FindPrecedingAssistant(history []model.AgentMessage) *model.AgentMessage {
	for i := len(history) - 1; i >= 1; i-- {
		if history[i].Sender == "toolrunner" || history[i].Role == "toolrunner" || history[i].Type == "tool_result" {
			for j := i - 1; j >= 0; j-- {
				if history[j].Role == "assistant" {
					return &history[j]
				}
			}
			break
		}
	}
	return nil
}

// Finds the latest relevant non-tool, non-error, non-self message to include as UX/spec context
func FindLastRelevantContext(history []model.AgentMessage, skipAgentName string) *model.AgentMessage {
	for i := len(history) - 1; i >= 0; i-- {
		msg := history[i]
		// Skip empty content, errors, tool calls/results, and our own agent name
		if msg.Role == "assistant" && msg.Type != "chat" {
			continue
		}
		if msg.Type == model.TypeToolCall || msg.Type == model.TypeToolResult || msg.Type == model.TypeError {
			continue
		}
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		// Optionally: only include specific roles/types (e.g. "ux", "user")
		// if msg.Role == "ux" || msg.Role == "user" {
		//     return &msg
		// }
		// Or just return first with good content
		return &msg
	}
	return nil
}

// NEW: Helper that returns only the fields you want in context (avoid nested history).
func filterContextForNext(ctx map[string]interface{}) map[string]interface{} {
	if ctx == nil {
		return nil
	}
	out := make(map[string]interface{}, len(ctx))
	for k, v := range ctx {
		// filter keys that shouldn't be recursively copied (e.g. "history")
		if k == "history" {
			continue // We'll add flat history separately
		}
		out[k] = v
	}
	return out
}

// Ensures that all newlines inside the "content" field of LLM JSON output are escaped as '\n'
func FixContentFieldNewlines(jsonStr string) string {
	// Regex: match the "content": "<string>"
	re := regexp.MustCompile(`("content"\s*:\s*")((?:[^"\\]|\\.)*)(")`)
	return re.ReplaceAllStringFunc(jsonStr, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		// Unescape already-escaped \n (to avoid double-escaping)
		content := parts[2]
		content = strings.ReplaceAll(content, "\\n", "\n")
		// Escape all newlines to JSON-compliant '\n'
		content = strings.ReplaceAll(content, "\n", `\n`)
		return parts[1] + content + parts[3]
	})
}

func fixJsonString(s string) string {
	// Fix badly formatted JSON with multiline string
	// 1. Find "content": "<multi-line>"
	// 2. Replace newlines inside the value with \n
	fixed := s

	// Regex to find: "content": "<...multi-line...>"
	re := regexp.MustCompile(`"content"\s*:\s*"(.*?)",\s*("|\})`)
	fixed = re.ReplaceAllStringFunc(fixed, func(match string) string {
		// Extract inside string
		parts := strings.SplitN(match, ":", 2)
		if len(parts) < 2 {
			return match
		}
		// Remove trailing quote/comma
		val := strings.TrimSpace(parts[1])
		if strings.HasPrefix(val, "\"") {
			val = val[1:]
		}
		// Find closing quote (may be comma or closing brace)
		endIdx := strings.LastIndex(val, "\"")
		if endIdx == -1 {
			return match
		}
		contentStr := val[:endIdx]
		contentStr = strings.ReplaceAll(contentStr, "\n", "\\n") // Escape real newlines

		// Compose new string
		return fmt.Sprintf(`"content": "%s",`, contentStr)
	})
	return fixed
}

func injectError(msg model.AgentMessage, prompt string) string {
	var errBuf strings.Builder

	if errText, ok := msg.Context["tool_error"].(string); ok && errText != "" {
		errBuf.WriteString("PREVIOUS TOOL ERROR:\n")
		errBuf.WriteString(errText)
		errBuf.WriteString("\n\n")
	}
	if phase, ok := msg.Context["tool_phase"].(string); ok && phase != "" {
		errBuf.WriteString("Error Phase: ")
		errBuf.WriteString(phase)
		errBuf.WriteString("\n")
	}
	if cmd, ok := msg.Context["tool_command"].(string); ok && cmd != "" {
		errBuf.WriteString("Command: ")
		errBuf.WriteString(cmd)
		errBuf.WriteString("\n")
	}
	if output, ok := msg.Context["tool_output"].(string); ok && output != "" {
		errBuf.WriteString("Output/Error Log:\n")
		errBuf.WriteString(output)
		errBuf.WriteString("\n")
	}
	if toolName, ok := msg.Context["tool_name"].(string); ok && toolName != "" {
		errBuf.WriteString("Tool Name: ")
		errBuf.WriteString(toolName)
		errBuf.WriteString("\n")
	}
	if detail, ok := msg.Context["tool_error_detail"].(string); ok && detail != "" {
		errBuf.WriteString("Error Detail: ")
		errBuf.WriteString(detail)
		errBuf.WriteString("\n")
	}

	// Optionally: Add more fields as needed (tokens, etc.)

	if errBuf.Len() > 0 {
		utils.Logger.Debug().
			Str("agent", msg.Sender).
			Msg("Injecting previous tool error details into prompt")
		// prompt = fmt.Sprintf("The previous code/tool execution failed. Here are the details:\n%s\n", errBuf.String())
		prompt += errBuf.String()
	}
	return prompt
}

// Utility to avoid out-of-bounds in logging.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
