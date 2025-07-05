package agent

import (
	"sync"

	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
)

// ChatManager implements HistoryProvider interface
var _ HistoryProvider = (*ChatManager)(nil)

type ChatManager struct {
	name         string
	agents       map[string]Agent
	agentInputs  map[string]chan model.AgentMessage
	agentOutputs map[string]chan model.AgentMessage
	history      []model.AgentMessage
	mu           sync.Mutex
	input        chan model.AgentMessage
	output       chan model.AgentMessage
}

func NewChatManager(agents []Agent) *ChatManager {
	cm := &ChatManager{
		agents:       make(map[string]Agent),
		agentInputs:  make(map[string]chan model.AgentMessage),
		agentOutputs: make(map[string]chan model.AgentMessage),
		history:      []model.AgentMessage{},
		input:        make(chan model.AgentMessage, 8),
		output:       make(chan model.AgentMessage, 8),
	}
	for _, a := range agents {
		cm.agents[a.Name()] = a
		cm.agentInputs[a.Name()] = make(chan model.AgentMessage, 2)
		cm.agentOutputs[a.Name()] = make(chan model.AgentMessage, 2)
		go a.Start(cm.agentInputs[a.Name()], cm.agentOutputs[a.Name()])
	}
	return cm
}

func (m *ChatManager) Name() string            { return m.name }
func (m *ChatManager) Description() string     { return "ChatManager for agent coordination" }
func (m *ChatManager) SetDescription(d string) {}

func (m *ChatManager) RegisterAgent(agent Agent) {
	m.agents[agent.Name()] = agent
	inCh := make(chan model.AgentMessage, 16)
	outCh := make(chan model.AgentMessage, 16)
	m.agentInputs[agent.Name()] = inCh
	m.agentOutputs[agent.Name()] = outCh
	agent.Start(inCh, outCh)
	utils.Logger.Debug().Str("agent", agent.Name()).Msg("Registered agent with manager")
}

func (m *ChatManager) SendToAgent(agentName string, msg model.AgentMessage) {
	inCh, ok := m.agentInputs[agentName]
	if ok {
		inCh <- msg
	} else {
		utils.Logger.Warn().Str("agent", agentName).Msg("No input channel for agent")
	}
}

func (m *ChatManager) InputChan() chan model.AgentMessage  { return m.input }
func (m *ChatManager) OutputChan() chan model.AgentMessage { return m.output }
func (m *ChatManager) AgentInputChan(agentName string) chan model.AgentMessage {
	return m.agentInputs[agentName]
}
func (m *ChatManager) AgentOutputChan(agentName string) chan model.AgentMessage {
	return m.agentOutputs[agentName]
}

// The main message loop (MAJOR CHANGES HERE)
func (m *ChatManager) Start() {
	go func() {
		for msg := range m.input {
			metrics.AgentMessagesTotal.WithLabelValues(m.Name()).Inc()
			utils.Logger.Debug().Str("sender", msg.OriginAgent).Msgf("Manager received message: %s", msg.Content)
			m.appendHistory(msg)

			var targetAgent string

			// ROUTE: Always send first user message to orchestrator!
			if msg.RouteTarget != "" && msg.RouteTarget != m.Name() {
				targetAgent = msg.RouteTarget
			} else if msg.Role == "user" {
				targetAgent = "Orchestrator"
			} else {
				targetAgent = "Orchestrator"
			}

			utils.Logger.Debug().Str("target", targetAgent).Msgf("---> Routing to %s", targetAgent)
			m.SendToAgent(targetAgent, msg)
			resp := <-m.AgentOutputChan(targetAgent)

			preview := resp.Content
			if len(preview) > 50 {
				preview = preview[:50]
			}
			utils.Logger.Debug().Str("sender", resp.OriginAgent).Msgf("Manager received output response from %s: %s", targetAgent, preview)
			m.appendHistory(resp)

			// *** Handle reply chaining ***
			if resp.RouteTarget != "" && resp.RouteTarget != m.Name() {
				utils.Logger.Debug().Str("next_agent", resp.RouteTarget).Msgf("Forwarding response to next agent: %s", resp.RouteTarget)
				if resp.Context == nil {
					resp.Context = map[string]interface{}{}
				}
				resp.Context["history"] = m.history // inject up-to-date history
				m.input <- resp // enqueue next step
			} else {
				utils.Logger.Debug().Msg("No next agent specified, finalizing response")
				m.output <- resp // finally output if no next step
			}
		}
	}()
}

func (m *ChatManager) Start2() {
	go func() {
		for msg := range m.input {
			metrics.AgentMessagesTotal.WithLabelValues(m.Name()).Inc()
			utils.Logger.Debug().Str("sender", msg.OriginAgent).Msgf("Manager received message: %v", len(msg.Content))
			m.appendHistory(msg)

			var targetAgent string

			if msg.RouteTarget != "" && msg.RouteTarget != m.Name() {
				// If a route_target is specified, go directly there
				targetAgent = msg.RouteTarget
			} else if msg.Role == "user" {
				// User initial message, route to orchestrator and also append to history
				utils.Logger.Debug().Msg("Routing initial user message to Orchestrator")
				targetAgent = "Orchestrator"
				m.appendHistory(msg) // Store user message in history as first entry
			} else {
				// Fallback: orchestrator decides
				targetAgent = "Orchestrator"
			}

			utils.Logger.Debug().Str("target", targetAgent).Msgf("---> Routing to %s", targetAgent)
			m.SendToAgent(targetAgent, msg)
			resp := <-m.AgentOutputChan(targetAgent)
			preview := resp.Content
			if len(preview) > 10 {
				preview = preview[:min(50, len(preview))]
			}
			utils.Logger.Debug().Str("sender", resp.OriginAgent).Msgf("Manager received output response from %s: %s", targetAgent, preview)
			m.appendHistory(resp)

			// *** Now, handle reply chaining ***
			if resp.RouteTarget != "" && resp.RouteTarget != m.Name() {
				utils.Logger.Debug().Str("next_agent", resp.RouteTarget).Msgf("Forwarding response to next agent: %s", resp.RouteTarget)
				if resp.Context == nil {
					resp.Context = map[string]interface{}{}
				}
				// Remove history injection - agents will request it from manager instead
				// History stays centralized in manager to avoid duplication

				// Forward immediately to next agent
				m.input <- resp // enqueue next step
			} else {
				utils.Logger.Debug().Msg("No next agent specified, finalizing response")
				m.output <- resp // finally output if no next step
			}
		}
	}()
}

// Filter orchestrator messages from history (ADD THIS)
func filterHistoryNoOrchestrator(hist []model.AgentMessage) []model.AgentMessage {
	filtered := []model.AgentMessage{}
	for _, msg := range hist {
		if msg.Role != "orchestrator" {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

func (m *ChatManager) appendHistory(msg model.AgentMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history = append(m.history, msg)
}

// Add method to get history on demand
func (m *ChatManager) GetHistory() []model.AgentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return BuildChatHistoryForManager(m.history)
}

// Add method to get history for specific agent (with filtering if needed)
func (m *ChatManager) GetHistoryForAgent(agentName string) []model.AgentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	history := BuildChatHistoryForManager(m.history)
	// Optional: filter history relevant to this agent
	return history
}

// Helper for manager (like BuildChatHistory above)
func BuildChatHistoryForManager(history []model.AgentMessage) []model.AgentMessage {
	flat := make([]model.AgentMessage, 0, len(history))
	for _, m := range history {
		copy := m
		copy.Context = nil
		flat = append(flat, copy)
	}
	return flat
}

// Generic role/route-based routing.
// (You can replace with config-driven routing if you want full runtime flexibility)
func (m *ChatManager) route(msg model.AgentMessage) string {
	// 1. Route by explicit route_target (set by orchestrator or LLM)
	if tgt := msg.RouteTarget; tgt != "" {
		return tgt
	}
	// 2. Route by role -> agent name mapping
	switch msg.Role {
	case "ux":
		return "UX Assistant"
	case "assistant":
		return "Assistant"
	case "tool":
		return "ToolRunner"
	case "orchestrator":
		return "Orchestrator"
	case "user":
		// Default for first user message
		return "Orchestrator"
	default:
		// Fallback (should not occur if workflow is defined)
		return "Assistant"
	}
}
