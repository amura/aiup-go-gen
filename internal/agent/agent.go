package agent

import "aiupstart.com/go-gen/internal/model"

type Agent interface {
	Name() string
	Description() string
	SetDescription(desc string)
	Start(input <-chan model.AgentMessage, output chan<- model.AgentMessage)
}