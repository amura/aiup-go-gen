// internal/agent/planner.go
package agent

import (
	"fmt"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/model"
)

type Researcher struct {
    name string
	llmClient llm.LLMClient
}

func NewResearcher(name string, llmClient llm.LLMClient) *Researcher {
    return &Researcher{name: name, llmClient: llmClient}
}

func (p *Researcher) Name() string {
    return p.name
}

func (p *Researcher) Start(input <-chan model.Message, output chan<- model.Message) {
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
                reply = llmResponse
            }

            // Send the plan (or error) out
            output <- model.Message{
                Sender:  p.name,
                Content: reply,
            }

          
        }
    }()
}