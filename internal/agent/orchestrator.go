// internal/agent/orchestrator.go
// Orchestrator: decides which agent next.
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
Your role is to read the user’s request and decide **which agent** should handle it next.

Given a user request, plan the required subtasks, and for each:
- If code must be generated, assign to the "Assistant" agent.
- If the next action is to execute a tool, send to the ToolRunnerAgent.
- If the code fails verification, send the error and original task back to "Assistant" for correction and retry.
- Repeat until the code runs successfully or user stops.

Reply ONLY with a JSON object in the format:
- To assign: {"agent": "<agent_name>", "subtask": "<task or code>"}
- To verify: {"tool": "docker_exec", "args": { "language": "...", "code": "...", ... }}

Agents:
%s

User's request:
"%s"
`

// MessageType additions for routing/direct
const (
    TypeRoute  model.MessageType = "route"
    TypeDirect model.MessageType = "direct"
)



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
    name      string
    manager   *ChatManager  // for sending route requests
    agentList    []Agent
    strategy  func(request model.Message, agents []Agent) int
	llmClient llm.LLMClient
}

func NewOrchestratorAgent(name string, manager *ChatManager, agentList []Agent, llmClient llm.LLMClient) *OrchestratorAgent {
    return &OrchestratorAgent{name: name, manager: manager, agentList: agentList, llmClient: llmClient}
}

func (o *OrchestratorAgent) Name() string { return o.name }

func (o *OrchestratorAgent) SetManager(manager *ChatManager) {
	o.manager = manager
}

func (o *OrchestratorAgent) Start(input <-chan model.Message, output chan<- model.Message) {
    go func() {
        for msg := range input {
			metrics.AgentMessagesTotal.WithLabelValues(o.Name()).Inc()
            utils.Logger.Debug().
                Str("agent", o.name).
                Str("event", "received_message").
                Msgf("Received: %s", msg.Content)
            agentListStr := ""
            for _, a := range o.agentList {
                agentListStr += fmt.Sprintf("- %s\n", a.Name())
            }
            prompt := fmt.Sprintf(orchestrationPrompt, agentListStr, msg.Content)
            llmResp, err := o.llmClient.Generate(prompt)
            if err != nil {
                fmt.Println("[Orchestrator LLM ERROR]:", err)
                output <- model.Message{
                    Sender:      o.name,
                    Content:     "[Orchestrator LLM ERROR]: " + err.Error(),
                    MessageType: model.TypeRoute,
                    RouteTarget: "Assistant",
                }
                continue
            }

            //    // -- Handle OpenAI ToolCalls (preferred) --
            //    if len(llmResp.ToolCalls) > 0 {
            //     for _, toolCall := range llmResp.ToolCalls {
            //         toolAgent := ToolNameToAgent(toolCall.Name)
            //         output <- model.Message{
            //             Sender:      o.name,
            //             MessageType: model.TypeRoute,
            //             RouteTarget: toolAgent,
            //             ToolCall: &tools.ToolCall{
            //                 Name:   toolCall.Name,
            //                 Args:   toolCall.Args,
            //                 Caller: o.name,
            //             },
            //             Content: fmt.Sprintf("Tool call for %s", toolCall.Name),
            //         }
            //     }
            //     continue
            // }
			//  // Try to parse as tool call
			//  var toolResp LLMOrchToolResponse
			//  if err := json.Unmarshal([]byte(llmResp.Content), &toolResp); err == nil && toolResp.Tool != "" {
			// 	 fmt.Printf("[Orchestrator] Routing tool call to: %s\n", toolResp.Tool)
			// 	 // Route to the agent/tool registered for that tool
			// 	 toolAgent := ToolNameToAgent(toolResp.Tool) // implement this lookup as needed
			// 	 output <- model.Message{
			// 		 Sender:      o.name,
			// 		 MessageType: model.TypeRoute,
			// 		 RouteTarget: toolAgent,
			// 		 ToolCall: &tools.ToolCall{
			// 			 Name: toolResp.Tool,
			// 			 Args: toolResp.Args,
			// 			 Caller: o.name,
			// 		 },
			// 		 Content: fmt.Sprintf("Tool call for %s", toolResp.Tool),
			// 	 }
			// 	 continue
			//  }

            var routeResp LLMOrchAgentResponse
            if err := json.Unmarshal([]byte(llmResp.Content), &routeResp); err != nil {
                // fallback to assistant
                routeResp.Agent = "Assistant"
                routeResp.Subtask = msg.Content
            }
            // Ask manager to route to the chosen agent
            output <- model.Message{
                Sender:      o.name,
                Content:     routeResp.Subtask,
                MessageType: model.TypeRoute,
                RouteTarget: routeResp.Agent,
            }
        }
    }()
}


func ToolNameToAgent(tool string) string {
    switch tool {
    case "docker_exec":
        return "ToolRunner" // Change to your docker agent name
    case "stripe_mcp":
        return "HITL" // Or the agent handling Stripe MCP
    // Add more as needed
    default:
        return "Assistant"
    }
}