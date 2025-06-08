package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"aiupstart.com/go-gen/internal/config"
	"aiupstart.com/go-gen/internal/utils"
	openai "github.com/sashabaranov/go-openai"
    "aiupstart.com/go-gen/internal/metrics"
)

type OpenAIClient struct {
	client *openai.Client
	model  string
	cfg *config.McpConfig
}

// OpenAILLMClient definition
type OpenAILLMClient struct {
    client *openai.Client
    tools  []openai.Tool // your full tool definitions (schema)
}

func NewOpenAIClient(apiKey, model string, cfg *config.McpConfig) *OpenAIClient {
	client := openai.NewClient(apiKey)
	return &OpenAIClient{
		client: client,
		model:  model,
		cfg:    cfg,
	}
}

func NewOpenAILLMClient(client *openai.Client, tools []openai.Tool) *OpenAILLMClient {
    return &OpenAILLMClient{client: client, tools: tools}
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

func (c *OpenAILLMClient) Generate(prompt string) (LLMResponse, error) {
	// ctx := context.Background()
	utils.Logger.Debug().Str("module", "llm").Msgf("Generating response with OpenAI model for prompt: %s", prompt)
	// tools := BuildOpenAIToolsFromConfig(c.cfg)
	// utils.Logger.Debug().Str("module", "llm").Msgf("Using tools: %v", tools)

	req := openai.ChatCompletionRequest{
        Model:   "gpt-4.1", // or your configured model
        Messages: []openai.ChatCompletionMessage{
            {Role: openai.ChatMessageRoleSystem, Content: prompt},
        },
        Tools:      c.tools,
        ToolChoice: "auto",
    }
    resp, err := c.client.CreateChatCompletion(context.Background(), req)

	if err != nil {
		utils.Logger.Error().Err(err).Str("module", "llm").Msg("Failed to generate response from OpenAI")
		return LLMResponse{}, fmt.Errorf("OpenAI API error: %w", err)
	}
    // After: resp, err := c.client.CreateChatCompletion(...)
    metrics.OpenAITokensTotal.WithLabelValues("prompt").Add(float64(resp.Usage.PromptTokens))
    metrics.OpenAITokensTotal.WithLabelValues("completion").Add(float64(resp.Usage.CompletionTokens))
    metrics.OpenAITokensTotal.WithLabelValues("total").Add(float64(resp.Usage.TotalTokens))
    utils.Logger.Debug().Str("module", "llm").Msgf("Token usage: prompt=%d, completion=%d, total=%d",
        resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

	if len(resp.Choices) == 0 {
		utils.Logger.Error().Str("module", "llm").Msg("No choices returned from OpenAI API")
		return LLMResponse{}, fmt.Errorf("no choices returned from OpenAI API")
	}
	utils.Logger.Debug().Str("module", "llm").Msgf("OpenAI response: %s", resp.Choices[0].Message.Content)
	// Prepare response
    llmResp := LLMResponse{
        Content: resp.Choices[0].Message.Content, // for narrative/fallback
        Tokens: &resp.Usage,
    }

    // Extract and parse tool calls if any
    for _, tc := range resp.Choices[0].Message.ToolCalls {
        var args map[string]interface{}
        if tc.Function.Arguments != "" {
            if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
                args = map[string]interface{}{
                    "_unparsed": tc.Function.Arguments,
                }
            }
        }
        llmResp.ToolCalls = append(llmResp.ToolCalls, LLMToolCall{
            Name: tc.Function.Name,
            Args: args,
        })
    }

    return llmResp, nil
}
