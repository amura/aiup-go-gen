package aiup_go_gen

// Message represents a single message in the conversation.
type Message struct {
    Role    string // Role of the message sender: "system", "user", or "assistant"
    Content string // Content of the message
}