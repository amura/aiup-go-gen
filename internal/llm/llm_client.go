package llm

// LLMClient defines the interface for interacting with different LLM providers.
type LLMClient interface {
	Generate(prompt string) (string, error)
}
