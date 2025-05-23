package autogen

import (
	"fmt"
)

// Agent defines the interface for an agent.
type Agent interface {
	Name() string
	Receive(message Message) (Message, error)
}

// AssistantAgent represents an AI assistant agent.
type AssistantAgent struct {
	name         string
	llm          LLMClient
	systemPrompt string
}

// NewAssistantAgent creates a new AssistantAgent.
func NewAssistantAgent(name string, llm LLMClient, systemPrompt string) *AssistantAgent {
	return &AssistantAgent{
		name:         name,
		llm:          llm,
		systemPrompt: systemPrompt,
	}
}

// Name returns the name of the assistant agent.
func (a *AssistantAgent) Name() string {
	return a.name
}

// Receive processes the incoming message and generates a response using the LLM.
func (a *AssistantAgent) Receive(message Message) (Message, error) {
	prompt := fmt.Sprintf("%s\nUser: %s\nAssistant:", a.systemPrompt, message.Content)
	response, err := a.llm.Generate(prompt)
	if err != nil {
		return Message{}, err
	}
	return Message{
		Role:    "assistant",
		Content: response,
	}, nil
}
