package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"aiupstart.com/go-gen/internal/agent"
	"aiupstart.com/go-gen/internal/config"
	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"

	"github.com/joho/godotenv"
)

const (
    RoleAssistant = "assistant"
    RoleUx = "ux"
    // ...
)

func main() {
	// utils.Logger.Debug().Str("module", "main").Msg("Starting AIUpStart Playground")

	ux_prompt :=`
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

execToolName :=  "docker_exec" //"docker_exec"

ui_coder_prompt := fmt.Sprintf(`

## Task
- You receive a user request to create for an Angular project.
- The UX agent will provide wireframes for you to follow which go with the user request.
- Code a secure Web application that meets the request.
- Using mainly the angular cli tool available, provide all necessary files (components, services, pipes, etc.) following best angular practices, placing each code file in separate code blocks as per the tool
- Where interaction with an API is required, use environment variables for the API base URL, and use mocks for the API calls when executing the code.
- Wait for the %s executor to report success or failure of launching the web app, and fix any errors or warnings accordingly.
- Keep your comments or reasoning explanatory text to absolute minimum.
- You must ensure the site uses SSL certificates and is secure by default.
- You must attempt to launch the web app once it has built using ng serve, and report any errors or warnings to the user.

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
- If the %s executor indicates an error (non-zero exit code), revise the entire code

For all code execution or bug fixing, use the %s tool.
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

`, execToolName, execToolName, execToolName)

	_ = godotenv.Load() // Loads .env file if present

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set the OPENAI_API_KEY environment variable.")
		return
	}

	metrics.StartMetricsServer(":2112")

	// cfg, _ := tools.LoadToolConfig("./tools.yaml")

	pwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	mcpCfgPath := filepath.Join(filepath.Dir(pwd), "mcp_tools.yaml")
	mcp_cfg, err := config.LoadConfig(mcpCfgPath)
	if err != nil {
		panic(err)
	}

	registry := tools.NewToolRegistry()

	for _, mcp := range mcp_cfg.McpTools {
		tool := &tools.GenericMcpTool{
			NameStr:        mcp.Name,
			Endpoint:       mcp.Endpoint,
			DescriptionStr: mcp.Description,
		}
		registry.Register(tool)
	}

	// --- OpenAI client and tools ---
	// oaClient := openai.NewClient(apiKey)
	openAITools := llm.BuildOpenAIToolsFromConfig(mcp_cfg)
	llmClient := llm.NewOpenAILLMClient(apiKey,"gpt-4o", openAITools)

	
	if execToolName == "docker_exec" {

		newDockerExec := tools.NewDockerExecTool("go-gen-", "angular-dev:latest")
		registry.Register(newDockerExec)
	}


	daggerConfig := tools.DaggerExecConfig{
		Image:         "",  // need to update to include file path to Dockerfile
		Workdir:       "/src",
		MountPath:     "/src",
		OutputPath:    "/src/dist",
		Timeout:       45 * time.Second,
		Env:           map[string]string{"API_BASE_URL": "..."},
		CopyToHostDir: "./dist",
	}

	daggerExec := tools.NewDaggerExecTool(
		"dagger_exec",
		"Dagger-based container executor for code and build",
		daggerConfig,
	)

	if execToolName == "dagger_exec" {

		registry.Register(daggerExec)
	}


	// Put your actual prompt strings here, or load from file/config.
	// ui_coder_prompt := "You are a coding agent. ..." // <--- put your actual UI coder prompt here
	// ux_prompt := "You are a UX agent. ..."          // <--- put your actual UX agent prompt here

	// --- Agents ---
	assistant := agent.NewAssistantAgent("assistant", RoleAssistant, llmClient, "You are the Assistant Agent responsible for coding a angular SPA acting on a user request", ui_coder_prompt, registry)
	assistant.SetDescription("UI Coding assistant")

	ux_assistant := agent.NewAssistantAgent("ux", RoleUx, llmClient, "You are an experienced 'UX Design Expert' agent.", ux_prompt, nil)
	ux_assistant.SetDescription("UX wireframe assistant")

	toolRunner := agent.NewToolRunnerAgent("toolrunner", registry)
	agents := []agent.Agent{assistant, toolRunner, ux_assistant}

	// --- Orchestrator/Manager setup ---
	workflow := map[string]string{
    "user": "ux",
    "ux": "assistant",
    "assistant": "tool",
    "tool": "assistant",
}
	var manager *agent.ChatManager
	orchestrator := agent.NewOrchestratorAgent("Orchestrator", manager, agents, llmClient, workflow)
	agentListWithOrch := append([]agent.Agent{orchestrator}, agents...)
	manager = agent.NewChatManager(agentListWithOrch)
	orchestrator.SetManager(manager)

	// Ensure cleanup of docker containers on exit
	defer func() {
		// if manager.ToolRunner != nil {
		// 	utils.Logger.Info().Msg("Cleaning up Docker containers before exit.")
		// 	if err := manager.ToolRunner.CleanupContainer(context.TODO()); err != nil {
		// 		utils.Logger.Warn().Err(err).Msg("Failed to cleanup Docker container(s)")
		// 	}
		// }
	}()

	// --- Start the chat loop ---
	first := model.AgentMessage{
		Role:    model.RoleUser,
		Sender:  "User",
		Content: "Create a new angular web app which has a main user login page.",
	}
	manager.Start()

	go func() { manager.InputChan() <- first }()

	for msg := range manager.OutputChan() {
		fmt.Printf("*** [%s][%s]: %s ***\n", msg.Role, msg.Sender, msg.Content)
		// add termination condition to avoid endless loop
		if msg.Role == model.RoleSystem && (msg.Content == "exit" || msg.Content == "done") {
			fmt.Println("Exiting loop by system command.")
			break
		}
	}
}

func toolEnabled(cfg *tools.ToolConfig, name string) bool {
	for _, t := range cfg.Tools {
		if t.Name == name && t.Enabled {
			return true
		}
	}
	return false
}

// Optionally: update your agent strategy
func SimpleStrategy(msg model.AgentMessage, agents []agent.Agent) int {
	// Route to Assistant if normal chat, to HITL if tool call, etc.
	if msg.Type == model.TypeToolCall {
		for i, a := range agents {
			if a.Name() == "HITL" {
				return i
			}
		}
	}
	// Default to assistant
	for i, a := range agents {
		if a.Name() == "Assistant" {
			return i
		}
	}
	return 0 // fallback
}