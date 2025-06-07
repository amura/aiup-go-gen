// internal/chat/manager.go
// always starts flow with Orchestrator, then routes based on message type.
package agent

import (
	"fmt"

	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
)

type ChatManager struct {
	// agents        []Agent

	agents        map[string]Agent // agent name -> Agent
    agentInputs   map[string]chan model.Message
    agentOutputs  map[string]chan model.Message

	// prompts       map[string]string
	// selector      func(int,[]Agent, model.Message, []model.Message, interface{}) int
	// lastAgentIdx  int
	history       []model.Message
	// context       map[string]interface{}
	input, output chan model.Message
    // for limiting the number of iterations
    turns       int
	tokenCount  int
	maxTurns    int // e.g. 15
	maxTokens   int // e.g. 20000
    dockerContainerPrefix string
}

func NewChatManager(agentList []Agent) *ChatManager {
    agents := make(map[string]Agent)
    agentInputs := make(map[string]chan model.Message)
    agentOutputs := make(map[string]chan model.Message)
    for _, a := range agentList {
        utils.Logger.Debug().Str("agent","chatmanager").Msgf("Registering agent: %s", a.Name())
        agents[a.Name()] = a
        agentInputs[a.Name()] = make(chan model.Message, 2)
        agentOutputs[a.Name()] = make(chan model.Message, 2)
        go a.Start(agentInputs[a.Name()], agentOutputs[a.Name()])
    }
    return &ChatManager{
        agents:       agents,
        agentInputs:  agentInputs,
        agentOutputs: agentOutputs,
        input:        make(chan model.Message, 4),
        output:       make(chan model.Message, 4),
        history:      []model.Message{},
        maxTurns:   5,
        maxTokens:  20000,
        turns:      0,
        tokenCount: 0,
    }
}

// Start initializes the ChatManager by setting up dedicated input and output channels
// for each agent and launching their processing goroutines. It also starts a manager
// goroutine that listens for incoming messages, updates the conversation history,
// selects the next agent to handle the message using the selector strategy, forwards
// the message to the chosen agent, receives the agent's response, updates the history,
// and sends the response to the output channel. This enables concurrent, coordinated
// communication between the manager and multiple agents.
func (cm *ChatManager) Start() {
	utils.Logger.Debug().Msg(fmt.Sprintf("Starting ChatManager with agents: %d", len(cm.agents)))

    go func() {
		for {
			msg := <-cm.input
			utils.Logger.Debug().
				Str("sender", msg.Sender).
				Msgf("Manager received message: %s, now routing to [Orchestrator]", msg.Content)
			metrics.AgentMessagesTotal.WithLabelValues("Manager").Inc()
			cm.history = append(cm.history, msg)
			
			// Start by always sending to Orchestrator
			cm.agentInputs["Orchestrator"] <- msg
			resp := <-cm.agentOutputs["Orchestrator"]
			utils.Logger.Debug().
				Str("sender", resp.Sender).
				Msgf("Manager received response: %s", resp.Content)

			// Loop: keep routing until output is final
			for {

                // --- Limit enforcement ---
                if cm.turns >= cm.maxTurns {
					utils.Logger.Warn().
						Msgf("[ChatManager] Cycle limit reached (%d turns) - halting conversation.", cm.maxTurns)
					cm.output <- model.Message{
						Sender:  "Manager",
						Content: fmt.Sprintf("Conversation stopped: maximum of %d turns reached.", cm.maxTurns),
					}
					break
				}
				if cm.tokenCount >= cm.maxTokens {
					utils.Logger.Warn().
						Msgf("[ChatManager] Token limit reached (%d tokens) - halting conversation.", cm.maxTokens)
					cm.output <- model.Message{
						Sender:  "Manager",
						Content: fmt.Sprintf("Conversation stopped: maximum of %d tokens used.", cm.maxTokens),
					}
					break
				}

				// --- Tally turn count and tokens if LLM usage is returned ---
				cm.turns++
				if resp.Tokens != nil {
					cm.tokenCount += resp.Tokens.TotalTokens
					utils.Logger.Debug().
						Msgf("Token count updated: now at %d/%d tokens", cm.tokenCount, cm.maxTokens)
				}

				// --- Tool Call: Route to ToolRunner ---
				if resp.MessageType == model.TypeToolCall && resp.ToolCall != nil {
					toolAgent := ToolNameToAgent(resp.ToolCall.Name)
					utils.Logger.Debug().
						Str("tool", resp.ToolCall.Name).
						Msgf("Routing tool call to agent %s", toolAgent)

					if inChan, ok := cm.agentInputs[toolAgent]; ok {
						toolMsg := resp
                        // Set origin agent/content on tool call message
                        if toolMsg.OriginAgent == "" { toolMsg.OriginAgent = resp.Sender }
                        // Always store the *original* content if this is the first time
                        if toolMsg.OriginContent == "" {
                            // If Assistant was routed from Orchestrator, you may need to look one step back
                            if len(cm.history) > 0 {
                                // Use last content from history as best-effort
                                toolMsg.OriginContent = cm.history[len(cm.history)-1].Content
                            } else {
                                toolMsg.OriginContent = resp.Content
                            }
                        }
                        inChan <- toolMsg
						resp = <-cm.agentOutputs[toolAgent]
						continue // chain: check next response
					} else {
						utils.Logger.Error().
							Str("tool", resp.ToolCall.Name).
							Msgf("[ERROR] Unknown tool agent: %s", toolAgent)
						cm.output <- model.Message{Sender: "Manager", Content: "[ERROR] Unknown tool agent: " + toolAgent}
						break
					}
				}

                // --- Tool Result with error: route back to origin agent for repair ---
				if resp.MessageType == model.TypeToolResult && resp.IsError {
					targetAgent := resp.OriginAgent
					utils.Logger.Warn().
						Str("target_agent", targetAgent).
						Msgf("ToolRunner returned error, routing back to original agent for fix.")
					if inChan, ok := cm.agentInputs[targetAgent]; ok {
						fixMsg := model.Message{
							Sender:      "Manager",
                            Content:     fmt.Sprintf("ERROR executing previous code:\n%s\n\nOriginal request: %s\n\nPlease fix and retry.",
                                resp.Content, resp.OriginContent),MessageType: model.TypeRoute,
							RouteTarget: targetAgent,
						}
						inChan <- fixMsg
						resp = <-cm.agentOutputs[targetAgent]
						continue // chain: check next response
					} else {
						cm.output <- model.Message{Sender: "Manager", Content: "[ERROR] Could not find origin agent: " + targetAgent}
						break
					}
				}

				// --- Route as instructed to agent (could be Assistant, etc) ---
				if resp.MessageType == model.TypeRoute {
					agentName := resp.RouteTarget
					utils.Logger.Debug().
						Str("task", resp.Content).
						Msgf("Routing task to agent %s", agentName)
					if inChan, ok := cm.agentInputs[agentName]; ok {
						inChan <- resp
						resp = <-cm.agentOutputs[agentName]
						continue // chain: check next response
					} else {
						utils.Logger.Error().
							Str("agent", agentName).
							Msgf("[ERROR] Unknown agent: %s", agentName)
						cm.output <- model.Message{Sender: "Manager", Content: "[ERROR] Unknown agent: " + agentName}
						break
					}
				}


				// --- Otherwise, just forward output ---
				utils.Logger.Debug().
					Str("sender", resp.Sender).
					Msgf("Final output from agent: %s", resp.Content)
				cm.output <- resp
				break
			}
		}
	}()
}

// func (cm *ChatManager) Send(msg model.Message) {
// 	cm.input <- msg
// }

// func (cm *ChatManager) Receive() model.Message {
// 	return <-cm.output
// }

// Send injects a message into the chat workflow.
func (cm *ChatManager) Send(msg model.Message) {
    cm.input <- msg
}

// Receive returns the next message output by the orchestrated agents.
func (cm *ChatManager) Receive() model.Message {
    return <-cm.output
}

// InputChan exposes the chat manager's input channel (for user proxies).
func (cm *ChatManager) InputChan() chan<- model.Message {
    return cm.input
}

// OutputChan exposes the chat manager's output channel (if needed).
func (cm *ChatManager) OutputChan() <-chan model.Message {
    return cm.output
}

func (m *ChatManager) AgentInputChan(name string) chan<- model.Message {
    return m.agentInputs[name]
}
func (m *ChatManager) AgentOutputChan(name string) <-chan model.Message {
    return m.agentOutputs[name]
}

// History returns a copy of the conversation history.
func (cm *ChatManager) History() []model.Message {
    return append([]model.Message(nil), cm.history...)
}

// type Manager struct {
//     agents []agent.AssistantAgent
// }

// func NewManager(agents []agent.AssistantAgent) *Manager {
//     return &Manager{agents: agents}
// }

// func (m *Manager) Start() {
//     input := make(chan model.Message)
//     output := make(chan model.Message)

//     // Start all agents
//     for _, ag := range m.agents {
//         ag.Start(input, output)
//     }

//     // todo pass in proper prompt input
//     // Example: Send initial message
//     input <- model.Message{Sender: "User", Content: "Find recent papers on LLM applications."}

//     // Listen for responses
//     go func() {
//         for msg := range output {
//             // Handle responses (e.g., log or further processing)
//             fmt.Printf("[%s]: %s\n", msg.Sender, msg.Content)
//         }
//     }()
// }
