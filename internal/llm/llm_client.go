package llm


type LLMToolCall struct {
    Name string
    Args map[string]interface{}
}

type LLMResponse struct {
    Content   string
    ToolCalls []LLMToolCall
}

// LLMClient defines the interface for interacting with different LLM providers.
type LLMClient interface {
	Generate(prompt string) ( LLMResponse , error)
}
