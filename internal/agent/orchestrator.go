// internal/agent/orchestrator.go
package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
)

const orchestrationPrompt = `
You are an orchestrator agent. Your job is to decide which agent from the following should handle the next user task:

Available agents:
%s

Given the following message:
"%s"

Respond in JSON as: {"agent": "<agent_name>", "subtask": "<task or subtask to pass to agent>"}
`

// MessageType additions for routing/direct
const (
    TypeRoute  model.MessageType = "route"
    TypeDirect model.MessageType = "direct"
)

type LLMOrchResponse struct {
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
            utils.Logger.Debug().Str("[Orchestrator]",  fmt.Sprintf("Received: %s\n", msg.Content))
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
            var routeResp LLMOrchResponse
            if err := json.Unmarshal([]byte(llmResp), &routeResp); err != nil {
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

// Simple heuristics for routing
func containsTool(s string) bool {
	return strings.Contains(s, "tool") || strings.Contains(s, "fetch_arxiv")
}
func containsCode(s string) bool {
	return strings.Contains(s, "code") || strings.Contains(s, "python")
}