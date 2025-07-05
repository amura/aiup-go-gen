// internal/agent/orchestrator.go
package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
)

const orchestrationPrompt = `
You are an orchestration agent for an AI multi-agent system.

** You do not have any use of tools or code execution, and only route messages between agents. **

Rules:
1. When a request involves ui design, you must always delegate the first request to a UX agent for wireframe before passing to coder for implementation.

All communication and handoff must use strict JSON in the format:
{ "role": "...", "type": "...", "content": "...", ... }

NEVER return free-form text. Only valid JSON objects, always with role and type, and no other text should be returned other than JSON otherwise the output is invalid.


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

// orchestrator.go (EDITS BELOW)

func (o *OrchestratorAgent) Start(input <-chan model.AgentMessage, output chan<- model.AgentMessage) {
	go func() {
		for msg := range input {
			utils.Logger.Debug().
				Str("agent", o.name).
				Str("event", "received_message").
				Msgf("Received: %s", msg.Content)

			// Build the context/history as needed
			// (Filter out orchestrator messages from history for LLM if needed)
			history := filterHistoryWithNoOrch(o.manager.GetHistory())

			// Build the prompt, pass context etc as before
			prompt := buildPrompt(o, msg, history)

			llmResp, err := o.llmClient.Generate(history, prompt, false)
			if err != nil {
				output <- model.AgentMessage{
					Role:    model.RoleOrchestrator,
					Type:    model.TypeError,
					Content: "[Orchestrator LLM ERROR]: " + err.Error(),
					Context: msg.Context,
				}
				continue
			}

			// Parse next routing instruction from LLM response
			var routeMsg model.AgentMessage
			if err := json.Unmarshal([]byte(llmResp.Content), &routeMsg); err != nil {
				utils.Logger.Error().Err(err).Str("llm_output", llmResp.Content).Msg("Failed to unmarshal Orchestrator LLM response")
				routeMsg = model.AgentMessage{
					Role:    model.RoleOrchestrator,
					Type:    model.TypeError,
					Content: "[Orchestrator LLM output not valid JSON AgentMessage]: " + err.Error(),
					Context: msg.Context,
				}
			}

			expectedNext := o.workflow[msg.Role]
			if expectedNext != "" && routeMsg.RouteTarget != expectedNext {
				utils.Logger.Warn().Msgf("Orchestrator LLM output did not match workflow; overriding route_target to %s", expectedNext)
				routeMsg.RouteTarget = expectedNext
			}

			utils.Logger.Debug().
				Str("agent", o.name).
				Str("event", "routing").
				Str("to", routeMsg.RouteTarget).
				Str("type", string(routeMsg.Type)).
				Msg("Orchestrator routing decision")

			// Forward explicit next step; if RouteTarget == "" assume "end" or handle as needed.
			output <- routeMsg
		}
	}()
}

// ---(ADD THIS)--- Filter out orchestrator messages from history
func filterHistoryWithNoOrch(history []model.AgentMessage) []model.AgentMessage {
	var filtered []model.AgentMessage
	for _, msg := range history {
		if strings.ToLower(msg.Role) != "orchestrator" {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// ---(ADD THIS)--- Build the LLM prompt from state
func buildPrompt(o *OrchestratorAgent, msg model.AgentMessage, history []model.AgentMessage) string {
	// Compose agent list for prompt
	agentListStr := ""
	for _, a := range o.agentList {
		agentListStr += fmt.Sprintf("- %s\t%s\n", a.Name(), a.Description())
	}
	// Render history as JSON (if you want, you could serialize differently)
	// historyStr, _ := json.Marshal(history)
	// Compose system message
	// orchestrationPrompt,
	return fmt.Sprintf(
`You are the Orchestrator agent.
All replies must be valid strict JSON: { "role": ..., "type": ..., "content": ..., "route_target": ... }

Your only job is to route messages to the right agent as per this workflow:

Workflow:
- user → ux
- ux → assistant
- assistant → toolrunner
- toolrunner → assistant

If the latest message is from "user", ALWAYS reply with route_target: "ux".
If the latest is from "ux", ALWAYS route to "assistant".
If from "assistant" and a tool call is needed, route to "toolrunner". 
If from "toolrunner", ALWAYS return to "assistant".

Agents: 
%s

Latest user message: %s


N.B You MUST pass through the user message as is, and not modify it with any additiona text or explanation. Just pass it through as is under the 'content' attribute!
`,
	
		agentListStr, msg.Content,
	)
}


func (o *OrchestratorAgent) StartOld(input <-chan model.AgentMessage, output chan<- model.AgentMessage) {
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
					Role:        model.RoleOrchestrator,
					Type:        model.TypeRoute,
					Sender:      o.name,
					Content:     msg.Content, // fmt.Sprintf("Routing from role %s to role %s as per workflow", currRole, nextRole),
					RouteTarget: nextRole,
					Context:     cloneContext(msg.Context),
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

			history := o.manager.GetHistory()
			latestMsg := model.SafeJSON(msg)
			historyJSON := model.SafeJSON(history)
			prompt := fmt.Sprintf(
				orchestrationPrompt,
				agentListStr,
				msg.Role,
				latestMsg,
				historyJSON,
			)

			llmResp, err := o.llmClient.Generate(history, prompt, false)
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
