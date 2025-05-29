// internal/chat/manager.go
package agent

import (
	"fmt"

	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
)

type ChatManager struct {
	agents        []Agent
	prompts       map[string]string
	selector      func(int,[]Agent, model.Message, []model.Message, interface{}) int
	lastAgentIdx  int
	history       []model.Message
	context       map[string]interface{}
	input, output chan model.Message
}

func NewChatManager(agents []Agent, prompts map[string]string, selector func(int,[]Agent, model.Message, []model.Message, interface{}) int) *ChatManager {
	return &ChatManager{
		agents:   agents,
		prompts:  prompts,
		selector: selector,
		history:  make([]model.Message, 0),
		context:  make(map[string]interface{}),
		input:    make(chan model.Message),
		output:   make(chan model.Message),
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
	// This code sets up concurrent communication between a manager and multiple agents, giving each agent its own dedicated input and output channels for safe, parallel message passing. This is a common Go pattern for orchestrating multiple worker goroutines.
	agentInputs := make([]chan model.Message, len(cm.agents))
	agentOutputs := make([]chan model.Message, len(cm.agents))
	for i := range cm.agents {
		agentInputs[i] = make(chan model.Message)
		agentOutputs[i] = make(chan model.Message)
		go cm.agents[i].Start(agentInputs[i], agentOutputs[i])
	}

	go func() {
		cm.lastAgentIdx = 0 // Could be configured

		for {
			msg := <-cm.input
			cm.history = append(cm.history, msg)
			nextIdx := cm.selector(cm.lastAgentIdx, cm.agents, msg, cm.history, cm.context)
			agentInputs[nextIdx] <- msg
			resp := <-agentOutputs[nextIdx]
			cm.history = append(cm.history, resp)
			cm.lastAgentIdx = nextIdx
			cm.output <- resp
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
