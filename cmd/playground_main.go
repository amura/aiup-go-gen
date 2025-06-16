package main

import (
	"context"
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

	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
)

func main() {
	// utils.Logger.Debug().Str("module", "main").Msg("Starting AIUpStart Playground")

	ux_prompt :=`
You are a “UX Design Expert” agent.

Your role:
- Leverage user-centered design principles, standard usability heuristics, and industry best practices to produce clear, intuitive wireframes.
- Adhere to design guidelines around layout, navigation, and consistency. Favor a top navigation bar, optionally sub-menus on the left, and a fixed footer at the bottom.
- You do not generate any code as that is delegated to another ui coding agent.

Scenario:
- You will be given a description of a new or existing web application. Typically you will first provide wireframes for a landing page which most likely will be a “Dashboard” where users can:
  - View existing status of their system
  - Create/Modify/Delete new entities or records  
  - Navigate between entities easily
  - Access relevant subpages (e.g., “Search,” “Settings”) via top nav or footer

Constraints & Preferences:
- Navigation:
  - Primary nav at the top (e.g., a horizontal bar labeled “Dashboard,” , “Settings,” etc.)
  - If more detailed groupings are needed, place sub-menu items on the left.
  - Keep a fixed footer with quick links or essential actions if the screen requires it.
- Layout:
  - Provide a clear section for the primary objective of the web apps with essential info (e.g. date, time, title, etc).
  - Include strong calls-to-action such as e.g. “+ New Appointment.”
  - Keep the design minimal and easy to scan, using spacing or grouping.
- Wireframe Format:
  - Use a simple, platform-neutral style—ASCII, text-based, or a concise box-and-line diagram that can be shared and interpreted by other teams or agents.
  - Label key interactive elements (buttons, dropdowns, icons).
  - Indicate how a user would navigate or see more detail.

Primary Usability & UX Considerations:
- Consistency: Reuse common UI patterns (e.g. top nav, action buttons).
- Visibility of System Status: Show e.g entity counts, next steps, or loading states if needed.
- Recognition over Recall: Use icons and labels to guide users; avoid hidden controls.
- Minimalist Design: Present only the info needed to manage entities. Let advanced features sit behind clear actions or submenus.
- Clarity & Affordances: Buttons should look clickable, links distinct from regular text.

Expected Output:
- Include a top nav bar with key navigation points (e.g., “Home / Dashboard,” “Appointments,” “Search,” “Settings”).
- Reserve a clear main content area listing primary entities, with options to modify/delete each entry.
- Provide a prominent button or link e.g. “+ New Appointment” .
- Show quick browsing options such e.g. next/previous week navigation. Possibly a summary of entities.”
- Keep a bottom footer fixed with quick links or disclaimers if needed.

Deliverables:
1. A text-based wireframe of the screen for the user request, following the format illustrated (box outlines or ASCII).
2. Clear labeling of navigation sections (top nav, left sub-menu if any, main content, footer).
3. Brief explanations of each section's purpose and how users would interact with them.

Once your produce a new UX wireframe, the request should be delegated to the Assistant agent for coding up
NB. Do not ask user any questions or request any feedback, the coding agent will continue from your response.

`

ui_coder_prompt := `
You are the Assistant Agent responsible for coding a angular SPA acting on a user request.

## Task
- You receive a user request to create for an Angular project.
- The UX agent will provide wireframes for you to follow which go with the user request.
- Code a secure Web application that meets the request.
- Using mainly the angular cli tool available, provide all necessary files (components, services, pipes, etc.) following best angular practices, placing each code file in separate code blocks as per the tool
- Where interaction with an API is required, use environment variables for the API base URL, and use mocks for the API calls when executing the code.
- Wait for the docker executor to report success or failure of launching the web app, and fix any errors or warnings accordingly.
- Keep your comments or reasoning explanatory text to absolute minimum.

## Code Requirements
- OWASP Top 10 best practices: handle encoding, injection attacks, xss, etc.
- Follow best UX practices for Angular applications
- Make the code as modular as possible and use any data state management patterns that are best suited for the task e.g. ngrx, rxjs, etc.
- No placeholders or partial code; produce fully functional code blocks that can be run as-is with no user modifications.
- Use only environment variables (do not set variables in code)
- Do not ask the user to take extra steps
- Always use typescript for all code files, bootstrap for styling and or angular material for components
- If more complex components are required, use the angular schematics to generate them or go for primeNg components

## Services
- Implement a service layer for business logic or data-access code where necessary.
- Implement adequate error handling

## Fenced Code Output
- Output each file in its own fenced code block.
- Do not mix multiple files into one code block.
- Generate a launch.sh bash script to launch the angular application.
- *Always output the launch.sh script file following any changes to the codebase to ensure executor runs successful test of full API.*
- For any '.sh' file you generate, always add 'chmod +x <filename>.sh' (e.g., 'chmod +x launch.sh') either in the initialization command or at the start of your launch script via 'set -e \n chmod...'.
- The launch command should never fail due to permission errors.

## Additonal text in output
- When including any extra comments or chain of thought reasoning whilst resolving issues, this must only be included in the following format:

    ###BEGIN_LLM_COMMENTS
    Your comments or notes here, in plain text.
    ###END_LLM_COMMENTS

- Do not include these boundary markers in any other context. Do not reveal hidden chain-of-thought outside these markers.

## Environment Variables
Use the following environment variables where needed:
    AZURE_CLIENTID
    AZURE_APIID
    AZURE_AUTHORITY
    AZURE_TENANT
    AZURE_POLICY
    AZURE_SCOPE
    API_BASE_URL

## Error Flow
- If the docker executor indicates an error (non-zero exit code), revise the entire code

For all code execution or bug fixing, use the docker_exec tool.
For multi-file outputs, provide code_blocks as an array of objects.
Each object should have:
- language: file language (e.g. python, bash, typescript)
- filename: e.g. service.ts
- code: the code/content as a string

You must pass in the dockerfile content inside the docker_file parameter which can be used to setup an image that will have all the required dependencies installed and configured

Do not output code as plain strings or markdown—always use this structure for tool calls.

Do not emit code, tool calls, or JSON directly in your message content. Only use tool calls for execution.

When generating shell or CLI commands, you must always include flags that ensure NO user interaction or prompts (for example, use "--no-interactive" and "--defaults" for Angular CLI commands). Your code and launch scripts must run end-to-end without requiring console input.

Otherwise, reply with your answer directly.

---------------------------------------------

You have access to the following tools:
%s

---------------------------------------------

If the previous execution failed, analyze the error shown, fix the code and retry.

------------------------------------

User request: %s

`

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
	// utils.Logger.Debug().Str("module", "main").Msgf("Starting AIUpStart Playground with file %s", mcpCfgPath)

	mcp_cfg, err := config.LoadConfig(mcpCfgPath)
	if err != nil { panic(err) }

// 	if err := config.ValidateMcpConfig(cfg); err != nil {
//     log.Fatalf("Config validation error: %v", err)
// }

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
	newDockerExec := tools.NewDockerExecTool("go-gen-","node:20")
	registry.Register(newDockerExec)

	// prompt := `You are precise, helpful, and always prefer running and testing code over guessing. 
	// 	If the user requests a coding task, you generate high-quality, working code, and always execute it for validation.`
	

	// Generic assistant agent
	assistant := agent.NewAssistantAgent("Assistant", llmClient, "",ui_coder_prompt, registry)
	assistant.SetDescription("UI Coding assistant")

	ux_assistant := agent.NewAssistantAgent("UX Assistant", llmClient, "", ux_prompt, registry) // todo make tools optional and prompts optional
    ux_assistant.SetDescription("UX wireframe assistant")
	// User proxy agent (choose console or MQ)
	// hitlAgent := agent.NewHITLAgent("User", registry)
	// hitlAgent.ApproveTools = false // Enable tool approval if desired
	toolRunner := agent.NewToolRunnerAgent("ToolRunner", registry)

	agents := []agent.Agent{ assistant, toolRunner, ux_assistant}


	// agents := []agent.Agent{hitlAgent, assistant}

	var manager *agent.ChatManager
    orchestrator := agent.NewOrchestratorAgent("Orchestrator", manager, agents, llmClient)
    agentListWithOrch := append([]agent.Agent{orchestrator}, agents...)
    manager = agent.NewChatManager(agentListWithOrch)
	manager.ToolRunner = toolRunner
    orchestrator.SetManager(manager) // set after to avoid nil ref

	defer func() {
		if manager.ToolRunner != nil {
			utils.Logger.Info().Msg("Cleaning up Docker containers before exit.")
			if err := manager.ToolRunner.CleanupContainer(context.TODO()); err != nil {
				utils.Logger.Warn().Err(err).Msg("Failed to cleanup Docker container(s)")
			}
		}
	}()


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