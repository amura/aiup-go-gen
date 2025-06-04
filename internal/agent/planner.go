// internal/agent/planner.go
package agent

import (
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/model"

	"fmt"
)

// Planner is an agent that plans task steps using an LLM client.
type Planner struct {
    name      string
    llmClient llm.LLMClient
}

func NewPlanner(name string, llmClient llm.LLMClient) *Planner {
    return &Planner{
        name:      name,
        llmClient: llmClient,
    }
}

func (p *Planner) Name() string { return p.name }

// Start launches the planner's asynchronous message loop.
func (p *Planner) Start(input <-chan model.Message, output chan<- model.Message) {
    go func() {
        for msg := range input {
            // Compose the LLM prompt based on incoming message
            prompt := fmt.Sprintf("As a planner agent, break down the following user task into actionable steps:\n\n%s", msg.Content)
            
            // Call the LLM client to generate a plan
            llmResponse, err := p.llmClient.Generate(prompt)
            var reply string
            if err != nil {
                reply = fmt.Sprintf("[Planner error]: %v", err)
            } else {
                reply = llmResponse.Content
            }

            // Send the plan (or error) out
            output <- model.Message{
                Sender:  p.name,
                Content: reply,
            }
        }
    }()
}