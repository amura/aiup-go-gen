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

Do not emit code or tool calls, or JSON directly in your message content. Only use tool calls for execution.

When generating shell or CLI commands, you must always include flags that ensure NO user interaction or prompts (for example, use "--no-interactive" and "--defaults" for Angular CLI commands). Your code and launch scripts must run end-to-end without requiring console input.

Otherwise, reply with your answer directly.
If the previous execution failed, analyze the error shown, fix the code and retry.

`

const defaultPersona = `
You are an AI coding assistant with expertise in software development, debugging, and tool integration.
`

const outputInstructions = `

- You cannot pass any instructions or explanations in your response as there is no user to read them. 
- You must provide all the code required to fulfil the user request and do not assume the user can make any changes to the code.
- All code provided must just work without any user interaction.
- You must not ask any questions or ask user for any clarifications or information, even after you manage to resolve some issues during execution.

* IMPORTANT: You MUST respond strictly with either pure JSON or an OpenAI-compatible tool call. Your response must be precisely formatted for JSON unmarshalling in Go. Do NOT include any additional text, comments, markdown characters, or backticks , as these will break Go's JSON parsing.

**Permitted Output Formats:**

### 1. JSON Response (No Tool Call Needed):
When the task does not require a tool call, output only this strict JSON structure:

T_B_T
{
	"role": "ux",
	"type": "wireframe",
	"content": "ASCII wireframe and detailed explanation here, clearly formatted without backticks or markdown characters.",
	"route_target": "assistant"
}
T_B_T

- **Guidelines:**
	- Escape all newlines within "content" as \n.
	- Replace or encode special characters to ensure JSON unmarshalling compatibility with Go.
	- Do NOT prepend or append any explanatory text, comments, or additional formatting.

### 2. OpenAI Tool Call (Tool Execution Needed):
When tool execution is necessary, respond strictly with an OpenAI-compatible tool call response format:

- Utilize the native OpenAI tool call structure.
- Provide no JSON wrapper, additional text, comments, or markdown formatting.

---

Adhering strictly to these guidelines ensures compatibility and successful unmarshalling within your Go application.

**Critical:**
- NEVER output any other content format, instructions, comments, code snippets, markdown, or scripts.
- ANY violation will cause parsing failure in Go. Stick strictly to these rules.



`

// HistoryProvider interface allows agents to get history from a central source
type HistoryProvider interface {
	GetHistoryForAgent(agentName string) []model.AgentMessage
}

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
	promptBuilder  func(model.AgentMessage) string
	toolAgentPrompt map[string]interface{}
	historyProvider HistoryProvider // Add history provider reference
}

func NewAssistantAgent(name string, role string, llmClient llm.LLMClient, persona string, promptOverload string, 
		promptBuilder func(model.AgentMessage) string, registry *tools.ToolRegistry, historyProvider HistoryProvider) *AssistantAgent {
	return &AssistantAgent{
		name:            name,
		role:            role,
		llmClient:       llmClient,
		toolRegistry:    registry,
		persona:         persona,
		promptOverload:  promptOverload,
		promptBuilder:   promptBuilder,
		toolAgentPrompt: make(map[string]interface{}), // Initialize as nil, will be set when tool calls are made
		historyProvider: historyProvider,
	}
}

func (a *AssistantAgent) Name() string                                { return a.name }
func (a *AssistantAgent) Description() string                         { return a.description }
func (a *AssistantAgent) SetDescription(desc string)                  { a.description = desc }
func (a *AssistantAgent) SetHistoryProvider(provider HistoryProvider) { a.historyProvider = provider }

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
				Msgf("Received message from: %s", msg.Sender)

			metrics.AgentMessagesTotal.WithLabelValues(a.name).Inc()
			// utils.LogContext(msg.Context, "Assistant input context")

			var prompt string = `Resolve any issues with the tool execution and continue to fulfil the original user request with any follow up information provided. 
			You must not ask any questions or ask user for any clarifications or information, even after you manage to resolve some issues during execution.
			Your job is to produce complete code, so if you have addressed some issues, you must continue with tool use until the user request is fully resolved.
			` 

				// --- 3. Collect "history" for LLM. This is now always in msg.Context["history"] if present ---
			var history []model.AgentMessage
			// Manager is responsible for keeping/forwarding the full message array, minus orchestrator if needed.
			if h, ok := msg.Context["history"].([]model.AgentMessage); ok && len(h) > 0 {
				history = filterOutOrchestratorMessages(h) // See helper below
			} else {
				history = []model.AgentMessage{msg}
			}

			// --- 4. Call LLM ---
			filteredHistory := filterAgentHistory(history)

			if msg.ToolCall == nil {

			// --- 1. Prepare prompt ---
		    prompt = a.buildUpPrompt(msg.Content) // Use the local buildUpPrompt method
			// if a.promptOverload != "" {
			// 	prompt = a.promptOverload
			// 	utils.Logger.Debug().Msg("Using prompt override for AssistantAgent")
			// } else {
			// 	// Use default template, inject persona/tools, user request
			// 	// prompt = fmt.Sprintf(
			// 	// 	strings.ReplaceAll(assistantPromptTemplate, "T_B_T", "```"),
			// 	// 	a.personaOrDefault(),
			// 	// 	defaultPrompt,
			// 	// 	a.toolRegistry.DescribeTools(),
			// 	// 	msg.Content,
			// 	// )
			// 	prompt = a.buildUpPrompt(msg.Content) // Use the local buildUpPrompt method
			// }

			// --- 2. Inject tool error details into the prompt if present ---
			prompt = injectError(msg, prompt) // (helper function provided elsewhere; should append error detail if present)

			}

			utils.Logger.Debug().
				Str("agent", a.name).
				Msgf("About to call llm with prompt: %s", prompt[:min(100, len(prompt))])

		
			llmResp, err := a.llmClient.Generate(filteredHistory, prompt, true)
			if err != nil {
				utils.Logger.Error().Err(err).Msgf("LLM generation error")
				output <- model.AgentMessage{
					Role:    model.RoleAssistant,
					Type:    model.TypeError,
					Content: "[LLM ERROR] " + err.Error(),
					Context: filterContextForNext(msg.Context),
				}
				continue
			}
			utils.Logger.Debug().Str("llm_response", fmt.Sprintf("%v", len(llmResp.Content))).Msg("LLM response received!!")

			// --- 5. Handle tool calls ---
			if len(llmResp.ToolCalls) > 0 {
				utils.Logger.Debug().Int("tool_calls_count", len(llmResp.ToolCalls)).Msg("LLM response contains tool calls")
				for _, tc := range llmResp.ToolCalls {
					if tc.ID == "" {
						tc.ID = uuid.New().String() // Or whatever you use for unique IDs
					}
					output <- model.AgentMessage{
						Role:       model.RoleAssistant,
						Type:       model.TypeToolCall,
						ToolCall:   &common.ToolCall{Name: tc.Name, Args: tc.Args, Caller: a.name},
						ToolCallID: tc.ID, // Use the LLM's tool call ID
						Context:    msg.Context,
						RouteTarget: "toolrunner", // TODO: Replace with dynamic route if your registry can handle it.
					}
				}
				continue
			}

			utils.Logger.Debug().Msg("LLM response does not contain tool calls")

			// --- 6. Unmarshal output as JSON AgentMessage ---
			var replyMsg model.AgentMessage
			fixed := FixContentFieldNewlines(llmResp.Content)
			replyPtr, err := HandleLLMResponse(fixed)
			if err != nil {
				utils.Logger.Error().Err(err).Msgf("Failed to parse LLM response as AgentMessage \nContent: %s", llmResp.Content)
			} else if replyPtr != nil {
				replyMsg = *replyPtr
			}

			output <- model.AgentMessage{
				Role:        model.RoleAssistant,
				Type:        model.TypeChat,
				Content:     replyMsg.Content,
				RouteTarget: replyMsg.RouteTarget,
				Context:     msg.Context,
				Tokens:      nil,
			}

			// --- 7. OPTIONAL: Add "exit" logic ---
			// e.g., if replyMsg.Type == model.TypeDone || some condition in replyMsg/llmResp, then break to avoid infinite loops.
			// if replyMsg.Type == model.TypeDone {
			//     utils.Logger.Info().Msg("Assistant agent finished the workflow. Exiting agent loop.")
			//     return
			// }
		}
	}()
}

// ExtractJSON parses JSON content enclosed by markers from the LLM response
func ExtractJSON(response string) ([]byte, error) {
    re := regexp.MustCompile(`---JSON START---\s*(\{.*?\})\s*---JSON END---`)
    match := re.FindStringSubmatch(response)

    if len(match) < 2 {
        return nil, fmt.Errorf("no valid JSON section found")
    }

    return []byte(match[1]), nil
}

// Usage example
func HandleLLMResponse(response string) (*model.AgentMessage, error) {
	var result model.AgentMessage
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		utils.Logger.Error().Err(err).Msgf("Failed to unmarshal JSON into AgentMessage: %s", response)
	
	} else {
		return &result, nil
	}

		jsonBytes, err := ExtractJSON(response)
		if err != nil {
			// Log full response for debugging
			utils.Logger.Error().Err(err).Msgf("Failed to extract JSON from LLM response: %s", string(jsonBytes))
		} else {
			// Try to unmarshal the extracted JSON
			if err := json.Unmarshal(jsonBytes, &result); err == nil {
				return &result, nil
			}
			utils.Logger.Error().Err(err).Msgf("Failed to unmarshal extracted JSON into AgentMessage: %s", string(jsonBytes))
		}
		return nil, err


}

// Helper: personaOrDefault
func (a *AssistantAgent) personaOrDefault() string {
	if a.persona != "" {
		return a.persona
	}
	return "You are the Assistant Agent responsible for coding and bugfixing."
}

// Helper: filter out Orchestrator messages from history (to avoid polluting chat for LLMs)
func filterOutOrchestratorMessages(history []model.AgentMessage) []model.AgentMessage {
	var out []model.AgentMessage
	for _, msg := range history {
		if strings.ToLower(msg.Sender) != "orchestrator" && msg.Role != model.RoleOrchestrator {
			out = append(out, msg)
		}
	}
	return out
}

// func (a *AssistantAgent) Start2(input <-chan model.AgentMessage, output chan<- model.AgentMessage) {
// 	go func() {
// 		for msg := range input {
// 			utils.Logger.Debug().
// 				Str("agent", a.name).
// 				Str("event", "received_message").
// 				Msgf("Received message from : %s", msg.Sender)

// 			metrics.AgentMessagesTotal.WithLabelValues(a.name).Inc()
// 			utils.LogContext(msg.Context, "Assistant received context")
	

			


// 			history := buildFilteredHistory(msg)
// 			utils.Logger.Debug().Msgf("Filtered history length: %d", len(history))

// 			// --- KEEP prompt override logic, but clarify ---
// 			var prompt string = ""

// 			filteredHistory := filterAgentHistory(history)

// 			if msg.ToolCall == nil {

// 				if a.promptBuilder != nil {
// 					// Use prompt builder function
// 					prompt = a.promptBuilder(msg)
// 				} else {
// 					// Fallback to local prompt builder
// 					prompt = a.buildUpPrompt(msg.Content)
// 					// prompt = "You are an assistant. Respond to the user."
// 				}
	
// 					// --- ADD: If previous tool error, inject all error details into the prompt ---
// 				// prompt = injectToolErrorDetails(msg, prompt)
	
// 					utils.Logger.Debug().
// 					Str("agent", a.name).
// 					Msgf("About to call llm with prompt: %s", prompt[:min(50, len(prompt))])
	
// 			}

// 			// Call LLM with structured history, not just a string!
// 			llmResp, err := a.llmClient.Generate(filteredHistory, prompt, true)

// 			if err != nil {
	

// 				output <- model.AgentMessage{
// 					Role:    a.role,
// 					Type:    model.TypeError,
// 					Content: "[LLM ERROR] " + err.Error(),
// 					Context: filterContextForNext(msg.Context), // Pass only relevant context
// 				}
// 				continue
// 			}
// 			utils.Logger.Debug().Str("llm_response", fmt.Sprintf("%v", len(llmResp.Content))).Msg("LLM response received!!")

// 			// Handle tool calls if present.
// 			if len(llmResp.ToolCalls) > 0 {
// 				utils.Logger.Debug().Int("tool_calls_count", len(llmResp.ToolCalls)).Msg("LLM response contains tool calls")
// 				// Add the current prompt, the current agent message, and tool name to a dictionary

// 				for _, tc := range llmResp.ToolCalls {
// 					if tc.ID == "" {
// 						tc.ID = uuid.New().String() // Or whatever you use for unique IDs
// 					}
// 					am := model.AgentMessage{
// 						Role: a.role,
// 						Type: model.TypeToolCall,
// 						ToolCallID: tc.ID, // Use the LLM's tool call ID
// 						ToolCall: &common.ToolCall{
// 							Name:   tc.Name,
// 							Args:   tc.Args,
// 							Caller: a.name,
// 							ID:     tc.ID, // Use the LLM's tool call ID
// 						},
// 						Context:     msg.Context,
// 						RouteTarget: "toolrunner", // todo improve via e.g. a.toolRegistry.GetRunnerAgentName(tc.Name) or let orch decide by setting blank
// 					}
// 					utils.Logger.Debug().
// 						Str("agent", a.name).
// 						Str("tool_call", tc.Name).
// 						Msgf("Generated tool call: %s with ID %s", tc.Name, tc.ID)
// 					a.toolAgentPrompt[tc.ID] = prompt // Use the agent name as the caller
// 					output <- am
// 				}
// 				continue
// 			}

// 			utils.Logger.Debug().Msg("LLM response does not contain tool calls")
// 			var replyMsg model.AgentMessage

// 			fixed := FixContentFieldNewlines(llmResp.Content)
// 			utils.Logger.Debug().
// 				Str("agent", a.name).Msg("About to parse LLM response content as AgentMessage")
// 			if err := json.Unmarshal([]byte(fixed), &replyMsg); err != nil {
// 				// handle error
// 				utils.Logger.Error().Err(err).Msgf("Failed to parse LLM response as AgentMessage \nContent: %s", llmResp.Content)
// 				replyMsg = model.AgentMessage{
// 					Role:    model.RoleAssistant,
// 					Type:    model.TypeChat,
// 					Content: llmResp.Content,
// 				}
// 			}

// 			utils.Logger.Debug().
// 				Str("agent", a.name).
// 				Str("event", "llm_response").
// 				Str("role", string(replyMsg.Role)).
// 				Str("type", string(replyMsg.Type)).
// 				Msg("LLM response processed")

// 			output <- model.AgentMessage{
// 				Role:        a.role,
// 				Type:        model.TypeChat,
// 				Content:     replyMsg.Content,
// 				RouteTarget: replyMsg.RouteTarget,
// 				Context:     msg.Context,
// 				Tokens:      nil,
// 			}

// 				// --- ADD: Optional exit logic, e.g., if replyMsg signals done ---
// 			if replyMsg.Type == model.TypeDone || strings.Contains(strings.ToLower(replyMsg.Content), "exit") {
// 				utils.Logger.Info().Msg("Exit criteria met, terminating assistant agent loop.")
// 				close(output)
// 				return
// 			}
// 		}
// 	}()
// }

// --- ADD: Helper to filter out Orchestrator messages from history ---
func buildFilteredHistory(msg model.AgentMessage) []model.AgentMessage {
	var history []model.AgentMessage
	if ctxHist, ok := msg.Context["history"].([]model.AgentMessage); ok {
		for _, h := range ctxHist {
			if strings.ToLower(h.Sender) == "orchestrator" || strings.ToLower(h.Role) == "orchestrator" {
				continue // skip orchestrator
			}
			history = append(history, h)
		}
	}
	// Always include current message at the end if it's not orchestrator
	if strings.ToLower(msg.Sender) != "orchestrator" && strings.ToLower(msg.Role) != "orchestrator" {
		history = append(history, msg)
	}
	return history
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
			if m.ToolCall != nil {
				if m.ToolCall.ID == "" {
					// check on m.ToolCallID and set this to m.ToolCall.ID
					if m.ToolCallID != "" {
						m.ToolCall.ID = m.ToolCallID // Use the ID from the message if present
					} else {
						// This is a bug in the code that generates tool calls
						utils.Logger.Error().Msg("Tool call message found in history with empty ID! This is a bug in the code that generates tool calls.")
						panic("tool_call message found in history with empty ID! You must fix the code that generates tool calls.")
					}
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

func  (a *AssistantAgent) buildUpPrompt(userRequest string) string {

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
// --- ADD: Inject ALL tool error details when present into prompt ---
func injectToolErrorDetails(msg model.AgentMessage, prompt string) string {
	ctx := msg.Context
	if ctx == nil {
		return prompt
	}
	if errVal, ok := ctx["tool_error"]; ok {
		prompt += "\n\nPREVIOUS TOOL ERROR:\n\n" + fmt.Sprintf("%v", errVal)
	}
	if errDetail, ok := ctx["tool_error_detail"]; ok {
		prompt += "\n\nError Detail: " + fmt.Sprintf("%v", errDetail)
	}
	if errPhase, ok := ctx["tool_phase"]; ok {
		prompt += "\n\nError Phase: " + fmt.Sprintf("%v", errPhase)
	}
	if errName, ok := ctx["tool_name"]; ok {
		prompt += "\n\nTool Name: " + fmt.Sprintf("%v", errName)
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
