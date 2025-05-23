package main

import (
	"aiupstart.com/go-gen/llm_clients"
	"fmt"
	"github.com/joho/godotenv"
	"os"
)

func main() {

	_ = godotenv.Load() // Loads .env file if present

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set the OPENAI_API_KEY environment variable.")
		return
	}
	// OpenAI Example
	openaiClient := llm_clients.NewOpenAIClient(apiKey, "gpt-4o")
	response, err := openaiClient.Generate("Hello, OpenAI!")
	if err != nil {
		fmt.Println("OpenAI Error:", err)
	} else {
		fmt.Println("OpenAI Response:", response)
	}

	// // Anthropic Example
	// anthropicClient := aiup_go_gen.NewAnthropicClient("your-anthropic-api-key", "claude-3")
	// response, err = anthropicClient.Generate("Hello, Claude!")
	// if err != nil {
	//     fmt.Println("Anthropic Error:", err)
	// } else {
	//     fmt.Println("Anthropic Response:", response)
	// }

	// // Meta Example
	// metaClient := aiup_go_gen.NewMetaClient("your-meta-api-key", "llama-3", "https://api.nebius.ai/v1")
	// response, err = metaClient.Generate("Hello, LLaMA!")
	// if err != nil {
	//     fmt.Println("Meta Error:", err)
	// } else {
	//     fmt.Println("Meta Response:", response)
	// }

	// // Grok Example
	// grokClient := aiup_go_gen.NewGrokClient("your-grok-api-key", "grok-3", "https://api.grok.com/v1")
	// response, err = grokClient.Generate("Hello, Grok!")
	// if err != nil {
	//     fmt.Println("Grok Error:", err)
	// } else {
	//     fmt.Println("Grok Response:", response)
	// }
}
