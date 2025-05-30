package chat

import (
	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/model"
)

func RoundRobinSelector() func(last int, agents []agent.Agent, msg model.Message, history []model.Message) int {
    return func(last int, agents []agent.Agent, msg model.Message, history []model.Message) int {
        if len(agents) == 0 {
            return 0 // fallback
        }
        return (last + 1) % len(agents)
    }
}
