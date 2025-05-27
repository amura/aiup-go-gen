package chat

import (
	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/model"
)

func RoundRobinSelector() StateSelector {
	return StateSelectorFunc(func(lastAgentIdx int, agents []agent.Agent, lastMsg model.Message, history []model.Message, ctx map[string]interface{}) int {
		return (lastAgentIdx + 1) % len(agents)
	})
}
