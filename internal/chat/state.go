// internal/chat/state.go
package chat

import (
	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/model"
)

// StateSelector defines how the next agent is chosen based on current state.
type StateSelector interface {
	NextAgent(
		lastAgentIdx int,
		agents []agent.Agent,
		lastMsg model.Message,
		history []model.Message,
		ctx map[string]interface{},
	) int
}

// StateSelectorFunc is an adapter to allow using ordinary functions as selectors.
type StateSelectorFunc func(int, []agent.Agent, model.Message, []model.Message, map[string]interface{}) int

func (f StateSelectorFunc) NextAgent(
	lastAgentIdx int,
	agents []agent.Agent,
	lastMsg model.Message,
	history []model.Message,
	ctx map[string]interface{},
) int {
	return f(lastAgentIdx, agents, lastMsg, history, ctx)
}
