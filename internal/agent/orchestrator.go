// internal/agent/orchestrator.go
package agent

import (
	"encoding/json"
	"fmt"
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
)

const orchestrationPrompt = `
You are an orchestration agent for an AI multi-agent system.
All communication and handoff must use strict JSON in the format:
{ "role": "...", "type": "...", "content": "...", ... }
NEVER return free-form text. Only valid JSON objects, always with role and type.

Guidelines:
- If the request involves UI or wireframes, send to the UX agent ("role": "ux").
- Once wireframes are complete, immediately hand off to the assistant agent ("role": "assistant").
- If code/tool execution is required, hand off to the appropriate tool agent ("role": "tool", type: "tool_call").
- On tool error, always send full context, wireframes, and previous error info back to the coding agent for repair.
- When delegating to another agent, include *all* previous relevant content as context in your output JSON.
- For all agent routing: reply as
  { "role": "orchestrator", "type": "route", "content": "<brief reason>", "route_target": "<agent name>", "context": {...} }

Agents available:
%s

Latest message (from %s):
%s

Full context history:
%s
`

// ----------- CHANGES: Workflow map is primary decision mechanism -------------
var DefaultWorkflow = map[string]string{
	"user":      "ux",        // User messages go to UX agent
	"ux":        "assistant", // UX agent output to Assistant
	"assistant": "tool",      // Assistant output to ToolRunner (if tool_call)
	"tool":      "assistant", // On error or finish, back to Assistant
}

type OrchestratorAgent struct {
	name        string
	description string
	manager     *ChatManager
	agentList   []Agent
	llmClient   llm.LLMClient
	workflow    map[string]string // role -> next role
}

func NewOrchestratorAgent(
	name string,
	manager *ChatManager,
	agentList []Agent,
	llmClient llm.LLMClient,
	workflow map[string]string,
) *OrchestratorAgent {
	if workflow == nil {
		workflow = DefaultWorkflow
	}
	return &OrchestratorAgent{
		name:      name,
		manager:   manager,
		agentList: agentList,
		llmClient: llmClient,
		workflow:  workflow,
	}
}

func (o *OrchestratorAgent) Name() string        { return o.name }
func (o *OrchestratorAgent) Description() string { return o.description }
func (o *OrchestratorAgent) SetDescription(desc string) {
	o.description = desc
}
func (o *OrchestratorAgent) SetManager(manager *ChatManager) {
	o.manager = manager
}

func (o *OrchestratorAgent) Start(input <-chan model.AgentMessage, output chan<- model.AgentMessage) {
	go func() {
		for msg := range input {
			metrics.AgentMessagesTotal.WithLabelValues(o.Name()).Inc()
			utils.LogContext(msg.Context, "Orchestrator received context")
			utils.Logger.Debug().
				Str("agent", o.name).
				Str("event", "received_message").
				Msgf("Received: %s", msg.Content)

			currRole := msg.Role

			// ----------- [1] Workflow map for routing -------------
			nextRole, found := o.workflow[currRole]
			if found {
				routeMsg := model.AgentMessage{
					Role:       model.RoleOrchestrator,
					Type:       model.TypeRoute,
					Sender:     o.name,
					Content:    msg.Content, // fmt.Sprintf("Routing from role %s to role %s as per workflow", currRole, nextRole),
					RouteTarget: nextRole,
					Context:    cloneContext(msg.Context),
					OriginAgent: o.name,
				}

				utils.Logger.Info().
					Str("from", currRole).
					Str("to", nextRole).
					Msg("Workflow-based route by orchestrator")
				output <- routeMsg
				continue
			}

			// ----------- [2] Fallback: Use LLM prompt to determine route -------------
			// Compose agent list description for LLM prompt
			agentListStr := ""
			for _, a := range o.agentList {
				agentListStr += fmt.Sprintf("- %s\t%s\n", a.Name(), a.Description())
			}

			history := o.manager.history
			latestMsg := model.SafeJSON(msg)
			historyJSON := model.SafeJSON(history)
			prompt := fmt.Sprintf(
				orchestrationPrompt,
				agentListStr,
				msg.Role,
				latestMsg,
				historyJSON,
			)

			llmResp, err := o.llmClient.Generate(history, prompt)
			if err != nil {
				utils.Logger.Error().Err(err).Msg("Orchestrator LLM error")
				output <- model.AgentMessage{
					Role:    model.RoleOrchestrator,
					Type:    model.TypeError,
					Sender:  o.name,
					Content: "[Orchestrator LLM ERROR]: " + err.Error(),
					Context: msg.Context,
				}
				continue
			}

			var routeMsg model.AgentMessage
			if err := json.Unmarshal([]byte(llmResp.Content), &routeMsg); err != nil {
				utils.Logger.Error().Err(err).Str("llm_output", llmResp.Content).Msg("Failed to unmarshal Orchestrator LLM response")
				routeMsg = model.AgentMessage{
					Role:    model.RoleOrchestrator,
					Type:    model.TypeError,
					Sender:  o.name,
					Content: "[Orchestrator LLM output not valid JSON AgentMessage]: " + err.Error(),
					Context: msg.Context,
				}
			}
			// Merge context just in case
			if routeMsg.Context == nil {
				routeMsg.Context = make(map[string]interface{})
			}
			for k, v := range msg.Context {
				routeMsg.Context[k] = v
			}

			utils.Logger.Debug().
				Str("agent", o.name).
				Str("event", "routing").
				Str("to", routeMsg.RouteTarget).
				Str("type", string(routeMsg.Type)).
				Msg("Orchestrator routing decision (via LLM)")

			output <- routeMsg
		}
	}()
}

// Utility to clone a map for context (avoid sharing pointer)
func cloneContext(orig map[string]interface{}) map[string]interface{} {
	if orig == nil {
		return nil
	}
	copy := make(map[string]interface{}, len(orig))
	for k, v := range orig {
		copy[k] = v
	}
	return copy
}