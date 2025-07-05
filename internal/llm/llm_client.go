package llm

import (
	"aiupstart.com/go-gen/internal/model"
	openai "github.com/sashabaranov/go-openai"
)

// LLMToolCall and LLMResponse for structured outputs
type LLMToolCall struct {
	Name string
	Args map[string]interface{}
	ID  string
}

type LLMResponse struct {
	Content   string
	ToolCalls []LLMToolCall
	Tokens    *openai.Usage
}

// LLMClient interface
type LLMClient interface {
	Generate(history []model.AgentMessage, prompt string,enableTools bool) (LLMResponse, error)
}

// // LLMClient defines the interface for all LLM backends.
// type LLMClient interface {
//     // The strict "role/content" model. Tools are configured at creation.
//     Chat(messages []openai.ChatCompletionMessage) (LLMResponse, error)
//     Generate(prompt string) (LLMResponse, error) // (for backwards compat)
// }