package chat

import (
	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/model"
)

func RoundRobinSelector() func(int, []agent.Agent, model.Message, []model.Message, interface{}) int {
	return StateSelectorFunc(func(lastAgentIdx int, agents []agent.Agent, lastMsg model.Message, history []model.Message, ctx interface{}) int {
		return (lastAgentIdx + 1) % len(agents)
	})
}
