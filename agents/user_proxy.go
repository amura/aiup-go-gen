package autogen

import (
	"fmt"
)

// UserProxyAgent acts as a proxy for the user, capable of executing code.
type UserProxyAgent struct {
	name           string
	codeExecutor   CodeExecutor
	humanInputMode string // "ALWAYS", "TERMINATE", "NEVER"
}

// NewUserProxyAgent creates a new UserProxyAgent.
func NewUserProxyAgent(name string, executor CodeExecutor, humanInputMode string) *UserProxyAgent {
	return &UserProxyAgent{
		name:           name,
		codeExecutor:   executor,
		humanInputMode: humanInputMode,
	}
}

// Name returns the name of the user proxy agent.
func (u *UserProxyAgent) Name() string {
	return u.name
}

// Receive processes the incoming message, executes code if present, and returns the result.
func (u *UserProxyAgent) Receive(message Message) (Message, error) {
	// Extract code blocks from the message content
	codeBlocks := ExtractCodeBlocks(message.Content)

	var executionResults string
	for _, block := range codeBlocks {
		result := u.codeExecutor.ExecuteCodeBlock(block)
		if result.Error != nil {
			executionResults += fmt.Sprintf("Error executing %s: %v\n", block.Filename, result.Error)
		} else {
			executionResults += fmt.Sprintf("Output of %s:\n%s\n", block.Filename, result.Output)
		}
	}

	return Message{
		Role:    "user_proxy",
		Content: executionResults,
	}, nil
}
