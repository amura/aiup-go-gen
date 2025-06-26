package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"aiupstart.com/go-gen/internal/config"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
)

// OpenAILLMClient holds the openai client and tool specs
type OpenAILLMClient struct {
	client *openai.Client
	model  string
	tools  []openai.Tool
}

// Constructor
func NewOpenAILLMClient(apiKey string, model string, tools []openai.Tool) *OpenAILLMClient {
	return &OpenAILLMClient{
		client: openai.NewClient(apiKey),
		model:  model,
		tools:  tools,
	}
}


func BuildOpenAIToolsFromConfig(cfg *config.McpConfig) []openai.Tool {
    var tools []openai.Tool
    for _, t := range cfg.McpTools {
        for _, op := range t.Operations {
            // Use op.Parameters directly if present, or default to empty schema
            params := map[string]interface{}{}
            if op.Parameters != nil {
                params = op.Parameters
            } else {
                // Fallback: minimum valid schema
                params = map[string]interface{}{
                    "type":       "object",
                    "properties": map[string]interface{}{},
                    "required":   []string{},
                }
            }

            fn := &openai.FunctionDefinition{
                Name:        op.Name,
                Description: op.Description,
                Parameters:  params,
            }
            tools = append(tools, openai.Tool{
                Type:     "function",
                Function: fn,
            })
        }
    }
    return tools
}

// Utility: map your roles to OpenAI's roles
func mapRole(role string) string {
	switch role {
	case model.RoleUser:
		return openai.ChatMessageRoleUser
	case model.RoleAssistant:
		return openai.ChatMessageRoleAssistant
	case model.RoleSystem:
		return openai.ChatMessageRoleSystem
	case model.RoleTool:
		return openai.ChatMessageRoleTool
	default:
		return openai.ChatMessageRoleUser
	}
}

// Utility: map your ToolCall/ToolResult into OpenAI's struct (if needed)
func agentMessageToOpenAIMsg(msg model.AgentMessage) openai.ChatCompletionMessage {
	// PATCH: handle tool_call with required .id field
	if msg.Type == model.TypeToolCall && msg.ToolCall != nil {
		// PATCH: Ensure ToolCall.ID is always set
		if msg.ToolCall.ID == "" {
			msg.ToolCall.ID = uuid.New().String() // PATCH: Assign UUID if missing
			// You could also propagate any previous tool ID if re-calling!
		}
		return openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleAssistant,
			Content: "",
			ToolCalls: []openai.ToolCall{
				{
					ID:   msg.ToolCall.ID, // PATCH: Set .ID!
					Type: "function",
					Function: openai.FunctionCall{
						Name:      msg.ToolCall.Name,
						Arguments: encodeArgs(msg.ToolCall.Args),
					},
				},
			},
		}
	}

	// PATCH: handle tool_result from tool/ToolRunner with ToolCallID matching previous call
	if msg.Type == model.TypeToolResult && msg.ToolCall != nil {
		return openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: msg.ToolCall.ID, // PATCH: Must match previous call!
			Name:       msg.ToolCall.Name,
			Content:    msg.Content,
		}
	}

	// PATCH: all other messages as before
	role := openai.ChatMessageRoleUser
	if msg.Role == model.RoleAssistant {
		role = openai.ChatMessageRoleAssistant
	} else if msg.Role == model.RoleOrchestrator {
		role = openai.ChatMessageRoleAssistant // treat orchestrator as assistant
	}
	return openai.ChatCompletionMessage{
		Role:    role,
		Content: msg.Content,
	}
	// msg := openai.ChatCompletionMessage{
	// 	Role:    mapRole(m.Role),
	// 	Content: m.Content,
	// }

	// // Tool Call (function/tool) - see OpenAI docs for full fields
	// if m.ToolCall != nil && m.ToolCall.Name != "" {
	// 	// --- Ensure ToolCall.ID is set. OpenAI REQUIRES THIS! ---
    //     if m.ToolCall.ID == "" {
    //         // Generate a unique ID (MUST match ToolResult reply)
    //         m.ToolCall.ID = uuid.New().String()
    //     }
	// 	// Map your ToolCall to OpenAI ToolCall struct
	// 	toolCallJson, _ := json.Marshal(m.ToolCall.Args)
	// 	msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
	// 		ID:   m.ToolCall.ID, 
	// 		Type: openai.ToolTypeFunction,
	// 		Function: openai.FunctionCall{
	// 			Name:      m.ToolCall.Name,
	// 			Arguments: string(toolCallJson),
	// 		},
	// 	})
	// }
	// // Tool Result (for responses from tools)
	// // Tool Result (for responses from tools)
    // if m.ToolResult != nil && m.ToolResult.Name != "" {
    //     // msg.ToolCallID = m.ToolResult.CallID // If you use this
    //     if s, ok := m.ToolResult.Output.(string); ok {
    //         msg.Content = s
    //     } else if m.ToolResult.Output != nil {
    //         bytes, _ := json.Marshal(m.ToolResult.Output)
    //         msg.Content = string(bytes)
    //     }
    // }

	// return msg
}

func encodeArgs(args map[string]interface{}) string {
	bytes, _ := json.Marshal(args)
	return string(bytes)
}

// Generate: takes full chat history, prompt, and calls OpenAI
func (c *OpenAILLMClient) Generate(history []model.AgentMessage, prompt string) (LLMResponse, error) {
	utils.Logger.Debug().Str("module", "llm").Msgf("Sending request to OpenAI model for prompt: \n%s\n", prompt)

	var messages []openai.ChatCompletionMessage

	// Optional: prepend system prompt
	if prompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: prompt,
		})
	}

	// Convert all AgentMessages to OpenAI format
	for _, m := range history {
		messages = append(messages, agentMessageToOpenAIMsg(m))
	}

	req := openai.ChatCompletionRequest{
		Model:      c.model,
		Messages:   messages,
		Tools:      c.tools,
		ToolChoice: "auto",
	}

	resp, err := c.client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		utils.Logger.Error().Err(err).Str("module", "llm").Msg("Failed to generate response from OpenAI")
		return LLMResponse{}, fmt.Errorf("OpenAI API error: %w", err)
	}

	metrics.OpenAITokensTotal.WithLabelValues("prompt").Add(float64(resp.Usage.PromptTokens))
	metrics.OpenAITokensTotal.WithLabelValues("completion").Add(float64(resp.Usage.CompletionTokens))
	metrics.OpenAITokensTotal.WithLabelValues("total").Add(float64(resp.Usage.TotalTokens))
	utils.Logger.Debug().Str("module", "llm").Msgf("****** Token usage: prompt=%d, completion=%d, total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

	if len(resp.Choices) == 0 {
		utils.Logger.Error().Str("module", "llm").Msg("No choices returned from OpenAI API")
		return LLMResponse{}, fmt.Errorf("no choices returned from OpenAI API")
	}
	msg := resp.Choices[0].Message
    preview := msg.Content
    if len(preview) > 10 {
            preview = preview[:10]
        }
	utils.Logger.Debug().Str("module", "llm").Msgf("OpenAI response received: %s", preview) // Log first 100 chars

	// Parse tool calls if present
	var toolCalls []LLMToolCall
	for _, tc := range msg.ToolCalls {
		var args map[string]interface{}
		if tc.Function.Name != "" && tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = map[string]interface{}{"_unparsed": tc.Function.Arguments}
			}
		}
		toolCalls = append(toolCalls, LLMToolCall{
			Name: tc.Function.Name,
			Args: args,
			ID:   tc.ID, // Use ID if you need to track calls
		})
	}

	return LLMResponse{
		Content:   msg.Content,
		ToolCalls: toolCalls,
		Tokens:    &resp.Usage,
	}, nil
}