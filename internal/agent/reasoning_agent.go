// File: reasoning_agent.go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/model"
	"github.com/google/uuid"
)

// ReasoningAgent handles LLM responses and emits structured tool calls to the output channel.
type ReasoningAgent struct {
	llmClient llm.LLMClient
	output    chan model.AgentMessage
	name      string
}

func NewReasoningAgent(llmClient llm.LLMClient, output chan model.AgentMessage, name string) *ReasoningAgent {
	return &ReasoningAgent{
		llmClient: llmClient,
		output:    output,
		name:      name,
	}
}

func (r *ReasoningAgent) ID() string {
	return r.name
}

func (r *ReasoningAgent) Handle(ctx context.Context, msg *model.AgentMessage) error {
	log.Printf("[ReasoningAgent] Handling message: %s", msg.Content)

	history := []model.AgentMessage{*msg}
	prompt := msg.Content

	llmResp, err := r.llmClient.Generate(history, prompt, false)
	if err != nil {
		return fmt.Errorf("LLM generate failed: %w", err)
	}

	var parsed model.AgentMessage
	err = json.Unmarshal([]byte(llmResp.Content), &parsed)
	if err == nil && parsed.Type == model.TypeToolCall && parsed.ToolCall != nil {
		parsed.RouteTarget = "execution-agent"
		parsed.Context = msg.Context
		r.output <- parsed
		return nil
	}

	log.Println("[ReasoningAgent] Fallback: trying to recover from unstructured LLM response.")
	fallback := planNextToolCommand(llmResp.Content, msg)
	if fallback == nil {
		return fmt.Errorf("could not interpret LLM response into actionable tool command")
	}

	r.output <- *fallback
	return nil
}

func planNextToolCommand(response string, original *model.AgentMessage) *model.AgentMessage {
	if len(response) < 10 {
		return nil
	}

	args := map[string]interface{}{
		"script": "launch.sh",
	}

	tc := &common.ToolCall{
		Name:   "docker_exec",
		Args:   args,
		Caller: "reasoning-agent",
	}

	return &model.AgentMessage{
		Role:        model.RoleAssistant,
		Type:        model.TypeToolCall,
		ToolCall:    tc,
		ToolCallID: fmt.Sprintf("%s-%s", original.OriginAgent, uuid.NewString()), // if you generate one
		RouteTarget: "toolrunner",
		Context:     original.Context,
	}
}
