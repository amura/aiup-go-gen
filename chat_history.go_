package aiup_go_gen

// ChatHistory manages a sequence of messages in a conversation.
type ChatHistory struct {
    Messages []Message // Slice of messages in the conversation
}

// NewChatHistory creates a new ChatHistory instance with an optional system prompt.
func NewChatHistory(systemPrompt string) *ChatHistory {
    history := &ChatHistory{}
    if systemPrompt != "" {
        history.AddSystemMessage(systemPrompt)
    }
    return history
}

// AddSystemMessage adds a system message to the chat history.
func (ch *ChatHistory) AddSystemMessage(content string) {
    ch.Messages = append(ch.Messages, Message{
        Role:    "system",
        Content: content,
    })
}

// AddUserMessage adds a user message to the chat history.
func (ch *ChatHistory) AddUserMessage(content string) {
    ch.Messages = append(ch.Messages, Message{
        Role:    "user",
        Content: content,
    })
}

// AddAssistantMessage adds an assistant message to the chat history.
func (ch *ChatHistory) AddAssistantMessage(content string) {
    ch.Messages = append(ch.Messages, Message{
        Role:    "assistant",
        Content: content,
    })
}

// GetMessages returns the slice of messages in the chat history.
func (ch *ChatHistory) GetMessages() []Message {
    return ch.Messages
}