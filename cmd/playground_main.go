package main

import (
	"fmt"
	"os"

	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/chat"
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/tools"
	// "aiupstart.com/go-gen/internal/utils"
	"github.com/joho/godotenv"
)

func main() {

	_ = godotenv.Load() // Loads .env file if present

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set the OPENAI_API_KEY environment variable.")
		return
	}

	// OpenAI Example
	llmClient := llm.NewOpenAIClient(apiKey, "gpt-4o")

	// Logger to file as well as stdout
	// f, _ := os.OpenFile("run.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// utils.Logger.SetOutput(f)

	// Tool config (optional)
	cfg, _ := tools.LoadToolConfig("config/tools.yaml")

	registry := tools.NewToolRegistry()
	// Register tools only if enabled
	if cfg == nil || toolEnabled(cfg, "fetch_arxiv") {
		registry.Register(&tools.FetchArxivTool{})
	}
	registry.Register(&tools.DockerExecTool{})

	prompt := `
	You are an AI assistant that can call external tools by outputting ONLY a JSON object in this format:

	{"tool": "fetch_arxiv", "args": {"query": "<search query>"}}

	If the user asks you to find papers, call the tool. DO NOT reply with natural language or explanation. Only output the JSON.

	User request: %s
	`

	// Generic assistant agent
	assistant := agent.NewAssistantAgent("Assistant", llmClient, prompt, registry)

	// User proxy agent (choose console or MQ)
	userProxy := agent.NewUserProxyAgent("User")

	agents := []agent.Agent{userProxy, assistant}
	selector := chat.RoundRobinSelector() // or advanced selector
	manager := chat.NewChatManager(agents, nil, selector)
	manager.Start()

	// Channel from user input to chat
	go userProxy.UserInputLoop(manager.InputChan())

	// Example: print everything that comes back to userProxy
	for {
		msg := manager.Receive()
		fmt.Printf("[System]: %s: %s\n", msg.Sender, msg.Content)
	}
	//   if cfg == nil || toolEnabled(cfg, "markdown_report") {
	// 	  registry.Register(&tools.MarkdownReportTool{})
	//   }

	// llmClient := llm.NewOpenAIClient("sk-your-openai-key")
	// planner := agent.NewPlanner("Planner", llmClient)
	// researcher := agent.NewResearcher("Researcher", llmClient)
	// // writer := agent.NewWriter("Writer", llmClient)

	// manager := chat.NewChatManager([]agent.AssistantAgent{planner, researcher})
	// manager.Start()

	// Keep the main function alive
	// select {}

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

func toolEnabled(cfg *tools.ToolConfig, name string) bool {
	for _, t := range cfg.Tools {
		if t.Name == name && t.Enabled {
			return true
		}
	}
	return false
}
