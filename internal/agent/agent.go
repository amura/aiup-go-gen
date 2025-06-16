package agent

import (

	"aiupstart.com/go-gen/internal/model"
)

// Agent defines the interface for an agent.
type Agent interface {
    Name() string
    Description() string
    Start(input <-chan model.Message, output chan<- model.Message)
}
