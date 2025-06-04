package main

import (
	"fmt"
	"os"
	"path/filepath"

	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/config"
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"

	// "aiupstart.com/go-gen/internal/utils"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
)

func main() {
	utils.Logger.Debug().Str("module", "main").Msg("Starting AIUpStart Playground")

	_ = godotenv.Load() // Loads .env file if present

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set the OPENAI_API_KEY environment variable.")
		return
	}

	metrics.StartMetricsServer(":2112")

	
	// Logger to file as well as stdout
	// f, _ := os.OpenFile("run.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	// utils.Logger.SetOutput(f)

	// Tool config (optional)
	cfg, _ := tools.LoadToolConfig("./tools.yaml")

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	mcpCfgPath := filepath.Join(filepath.Dir(pwd), "mcp_tools.yaml")
	utils.Logger.Debug().Str("module", "main").Msgf("Starting AIUpStart Playground with file %s", mcpCfgPath)

	mcp_cfg, err := config.LoadConfig(mcpCfgPath)
	if err != nil { panic(err) }

	registry := tools.NewToolRegistry()

	for _, mcp := range mcp_cfg.McpTools {
		tool := &tools.GenericMcpTool{
			NameStr:        mcp.Name,
			Endpoint:       mcp.Endpoint,
			DescriptionStr: mcp.Description,
		}
		registry.Register(tool)
	}

	// OpenAI Example
	// 
	// llmClient := llm.NewOpenAIClient(apiKey, "gpt-4o", mcp_cfg)

	oaClient := openai.NewClient(apiKey)
	openAITools := llm.BuildOpenAIToolsFromConfig(mcp_cfg)
	  // --- 4. Wrap OpenAI client in your LLM interface ---
	  llmClient := llm.NewOpenAILLMClient(oaClient, openAITools)

	//   // --- 5. Build ToolRegistry for runtime tool calls ---
	//   registry := tools.NewToolRegistry()
	//   for _, mcp := range cfg.McpTools {
	// 	  for _, op := range mcp.Operations {
	// 		  registry.Register(&tools.GenericMcpTool{
	// 			  NameStr:        op.Name,
	// 			  Endpoint:       mcp.Endpoint,
	// 			  DescriptionStr: op.Description,
	// 			  // Optionally: add validation fields for runtime here
	// 		  })
	// 	  }
	//   }



	// Register tools only if enabled
	if cfg == nil || toolEnabled(cfg, "fetch_arxiv") {
		registry.Register(&tools.FetchArxivTool{})
	}
	registry.Register(&tools.DockerExecTool{})

	prompt := `You are precise, helpful, and always prefer running and testing code over guessing. 
		If the user requests a coding task, you generate high-quality, working code, and always execute it for validation.`
	

	// Generic assistant agent
	assistant := agent.NewAssistantAgent("Assistant", llmClient, prompt, registry)

	// User proxy agent (choose console or MQ)
	// hitlAgent := agent.NewHITLAgent("User", registry)
	// hitlAgent.ApproveTools = false // Enable tool approval if desired
	toolRunner := agent.NewToolRunnerAgent("ToolRunner", registry)

	agents := []agent.Agent{ assistant, toolRunner}


	// agents := []agent.Agent{hitlAgent, assistant}

	var manager *agent.ChatManager
    orchestrator := agent.NewOrchestratorAgent("Orchestrator", manager, agents, llmClient)
    agentListWithOrch := append([]agent.Agent{orchestrator}, agents...)
    manager = agent.NewChatManager(agentListWithOrch)
    orchestrator.SetManager(manager) // set after to avoid nil ref


	// // check if hitlAgent is enabled via if check append(agents, hitlAgent)...
	// selector := chat.RoundRobinSelector() // or advanced selector

	// orchestrator := agent.NewOrchestratorAgent("Orchestrator", agents, SimpleStrategy)
	// topAgents := []agent.Agent{orchestrator}
	
	// manager := agent.NewChatManager(topAgents, chat.RoundRobinSelector())
	// manager.Start()
	
    first := model.Message{Sender: "User", Content: "Create a new angular web app which has a main user login page."}
    manager.Start()

    go func() { manager.InputChan() <- first }()


	// go hitlAgent.BeginChat(manager, first)

	for msg := range manager.OutputChan() {
		fmt.Printf("*** [%s]: %s ***\n", msg.Sender, msg.Content)
		// add termination condition to avoid endless loop
	}

    // for {
    //     msg := <-manager.OutputChan()
    //     fmt.Printf("[%s]: %s\n", msg.Sender, msg.Content)

    //     turns++
    //     if turns >= maxTurns {
    //         fmt.Println("Max turns reached. Exiting.")
    //         break
    //     }
    //     // Or terminate if an agent outputs an exit/done message
    //     lower := strings.ToLower(msg.Content)
    //     if strings.Contains(lower, "exit") || strings.Contains(lower, "done") {
    //         fmt.Println("Termination cue detected. Exiting.")
    //         break
    //     }
    //     // Autonomous: feed response back to manager.input for next agent
    //     manager.InputChan() <- msg
    // }
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

func SimpleStrategy(msg model.Message, agents []agent.Agent) int {
    // Route to Assistant if normal chat, to HITL if tool call, etc.
    if msg.MessageType == model.TypeToolCall {
        for i, a := range agents {
            if a.Name() == "HITL" { return i }
        }
    }
    // Default to assistant
    for i, a := range agents {
        if a.Name() == "Assistant" { return i }
    }
    return 0 // fallback
// }

// func BuildMcpPrompt(cfg *config.Config) string {
//     s := "You have access to these MCP tools:\n"
//     for _, t := range cfg.McpTools {
//         s += fmt.Sprintf("\n%s: %s\n", t.Name, t.Description)
//         s += "Supported operations:\n"
//         for _, op := range t.Operations {
//             s += fmt.Sprintf("- %s (%s %s): %s\n  Example: { \"tool\": \"%s\", \"args\": { \"path\": \"%s\", \"method\": \"%s\", \"body\": %v } }\n",
//                 op.Name, op.Method, op.Path, op.Description, t.Name, op.Path, op.Method, op.ExampleArgs)
//         }
//     }
//     s += "\nTo call a tool, always use this format:\n"
//     s += "{ \"tool\": \"tool_name\", \"args\": { \"path\": \"...\", \"method\": \"...\", \"body\": {...} } }\n"
//     s += "Do not use external APIs directly—always use these MCP tools.\n"
//     return s
// }
}