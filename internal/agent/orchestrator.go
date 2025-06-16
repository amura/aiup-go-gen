// internal/agent/orchestrator.go
package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"
)

const orchestrationPrompt = `
You are an orchestration agent for an AI multi-agent system.
Your job is to read the user's request and decide **which agent** should handle it next.

Guidelines:
- If the request involves UI or wireframes, FIRST send it to the "UX Assistant" for wireframe generation.
- Once wireframes are complete, IMMEDIATELY hand off the next step to the "Assistant" agent to generate code. Do NOT assign the same subtask to the same agent repeatedly.
- If code needs to be executed or validated, send it to the ToolRunnerAgent.
- If the code fails verification, send the error and original request back to the "Assistant" agent for correction and retry.
- When an agent's reply contains a clear handoff phrase such as "Requesting Assistant agent to code this UI in Angular", ALWAYS assign the next subtask to the agent mentioned in the handoff (e.g., "Assistant"). Never assign the same subtask to the same agent more than once unless it is a new, different subtask.
- Repeat until the task is completed or the user explicitly stops the process.

You MUST reply ONLY with a JSON object in one of the following formats:
- To assign: {"agent": "<agent_name>", "subtask": "<task or code to perform>"}
- To verify code: {"tool": "docker_exec", "args": { "language": "...", "code": "...", ... }}

Agents available:
%s

User's request:
"%s"
`

type LLMOrchToolResponse struct {
	Tool string                 `json:"tool"`
	Args map[string]interface{} `json:"args"`
}
type LLMOrchAgentResponse struct {
	Agent   string `json:"agent"`
	Subtask string `json:"subtask"`
}

// OrchestratorAgent handles planning and routing.
type OrchestratorAgent struct {
	name        string
	description string
	manager     *ChatManager
	agentList   []Agent
	strategy    func(request model.Message, agents []Agent) int
	llmClient   llm.LLMClient
	lastOutput  string
}

func NewOrchestratorAgent(name string, manager *ChatManager, agentList []Agent, llmClient llm.LLMClient) *OrchestratorAgent {
	return &OrchestratorAgent{name: name, manager: manager, agentList: agentList, llmClient: llmClient}
}

func (o *OrchestratorAgent) Name() string        { return o.name }
func (o *OrchestratorAgent) Description() string { return o.description }
func (o *OrchestratorAgent) SetManager(manager *ChatManager) {
	o.manager = manager
}

func (o *OrchestratorAgent) Start(input <-chan model.Message, output chan<- model.Message) {
	go func() {
		lastAgent := ""
		for msg := range input {
			metrics.AgentMessagesTotal.WithLabelValues(o.Name()).Inc()
			utils.Logger.Debug().
				Str("agent", o.name).
				Str("event", "received_message").
				Msgf("Received: %s", msg.Content)

			utils.LogContext(msg.Context, "Orchestrator received context")

			agentListStr := ""
			for _, a := range o.agentList {
				agentListStr += fmt.Sprintf("- %s \t %s \n", a.Name(), a.Description())
			}

			// [GENERIC]: Always pass forward ALL context!
			mergedContext := make(map[string]interface{})
			for k, v := range msg.Context {
				mergedContext[k] = v
			}

			prompt := fmt.Sprintf(orchestrationPrompt, agentListStr, msg.Content)
			llmResp, err := o.llmClient.Generate(prompt)
			if err != nil {
				utils.Logger.Error().
					Str("agent", o.name).
					Msgf("[Orchestrator LLM ERROR]: %v", err)
				output <- model.Message{
					Sender:      o.name,
					Content:     "[Orchestrator LLM ERROR]: " + err.Error(),
					MessageType: model.TypeRoute,
					RouteTarget: "Assistant",
					Context:     mergedContext,
				}
				continue
			}

			// Try tool call (function calling) FIRST!
			var toolResp LLMOrchToolResponse
			if err := json.Unmarshal([]byte(llmResp.Content), &toolResp); err == nil && toolResp.Tool != "" {
				utils.Logger.Debug().Msgf("Routing tool call to: %s", toolResp.Tool)
				toolAgent := ToolNameToAgent(toolResp.Tool)
				output <- model.Message{
					Sender:      o.name,
					MessageType: model.TypeRoute,
					RouteTarget: toolAgent,
					ToolCall: &tools.ToolCall{
						Name:   toolResp.Tool,
						Args:   toolResp.Args,
						Caller: o.name,
					},
					Content: fmt.Sprintf("Tool call for %s", toolResp.Tool),
					Context: mergedContext,
				}
				continue
			}

			// Otherwise: agent handoff
			var routeResp LLMOrchAgentResponse
			if err := json.Unmarshal([]byte(llmResp.Content), &routeResp); err != nil || routeResp.Agent == "" {
				// Fallback: check for classic handoff language
				handoff := false
				lowerContent := strings.ToLower(msg.Content)
				if strings.Contains(lowerContent, "requesting assistant agent") ||
					strings.Contains(lowerContent, "code this ui in angular") {
					handoff = true
				}
				if routeResp.Agent == lastAgent && handoff {
					utils.Logger.Warn().Msgf("Detected handover cue in message; overriding next agent to 'Assistant'")
					routeResp.Agent = "Assistant"
				}
				lastAgent = routeResp.Agent
				o.lastOutput = msg.Content // Always store full previous output!
				utils.Logger.Debug().Msgf("Passing previous agent's full output as subtask to %s", routeResp.Agent)
				output <- model.Message{
					Sender:      o.name,
					Content:     o.lastOutput,
					MessageType: model.TypeRoute,
					RouteTarget: routeResp.Agent,
					Context:     mergedContext,
				}
				continue
			}

			// Normal route: pass latest agent's full output
			o.lastOutput = msg.Content
			output <- model.Message{
				Sender:      o.name,
				Content:     o.lastOutput,
				MessageType: model.TypeRoute,
				RouteTarget: routeResp.Agent,
				Context:     mergedContext,
			}
		}
	}()
}

// ToolNameToAgent returns the agent for a given tool
func ToolNameToAgent(tool string) string {
	switch tool {
	case "docker_exec":
		return "ToolRunner"
	case "stripe_mcp":
		return "HITL"
	// Add more as needed
	default:
		return "Assistant"
	}
}