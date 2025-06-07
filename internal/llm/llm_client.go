package llm

import openai "github.com/sashabaranov/go-openai"


type LLMToolCall struct {
    Name string
    Args map[string]interface{}
}

type LLMResponse struct {
    Content   string
    ToolCalls []LLMToolCall
    Tokens    *openai.Usage 
}

// LLMClient defines the interface for interacting with different LLM providers.
type LLMClient interface {
	Generate(prompt string) ( LLMResponse , error)
}
