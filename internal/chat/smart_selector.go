package chat

import (
	"strings"

	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/model"
)

// Switch to Writer if Researcher output includes "report", else to Researcher, else fallback to Planner
func SmartSelector() func(int, []agent.Agent, model.Message, []model.Message, interface{}) int {
	return StateSelectorFunc(func(lastAgentIdx int, agents []agent.Agent, lastMsg model.Message, history []model.Message, ctx interface{}) int {
		// Example: you can use context, meta, or message content
		if strings.Contains(strings.ToLower(lastMsg.Content), "report") {
			// Find writer
			for i, ag := range agents {
				if ag.Name() == "Writer" {
					return i
				}
			}
		} else if strings.Contains(strings.ToLower(lastMsg.Content), "research") {
			for i, ag := range agents {
				if ag.Name() == "Researcher" {
					return i
				}
			}
		}
		// fallback
		return 0 // Planner
	})
}
