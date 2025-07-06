package main

import (
	"context"
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
	RoleUx        = "ux"
	// ...
)

func main() {
	// utils.Logger.Debug().Str("module", "main").Msg("Starting AIUpStart Playground")

	ux_prompt := `
Your role:

You are an experienced 'UX Design Expert' agent. Your task is to create wireframes for web applications based on user requests. You will produce text-based wireframes that can be easily interpreted by other agents or developers.

Your Responsibilities:
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

	execToolName := "docker_exec" //"docker_exec"

ui_coder_prompt := fmt.Sprintf(`
You have access to Angular CLI tools to create and modify Angular web applications based on user requests.

* IMPORTANT: Angular app can only be launched using the 'ng serve' command, and not by executing typescript files directly!! So DO NOT do things like 'sh src/app/login/login.component.ts! *

## Critical Response Rules
- NEVER output plain text instructions or explanations when tool execution is involved.
- ALWAYS use tool calls exclusively until the entire request is successfully executed.
- If a tool call results in an error, respond ONLY by correcting the error and immediately initiating another tool call. DO NOT provide intermediate explanations or plain text guidance.
- Follow a stepwise approach like use of cli tool for initial scaffolding, then on successful scaffold follow on with more tool calls to add / modify components and packages to fulfil the user request.
- There is no user available to read your instructions and perform any tasks on the code as they do not have access to the codebase, it's your job to use the tools to produce complete code that can be run end-to-end without any additional user interaction.

## Task
- Take a stepwise approach to building the angular application, and not attempt to do everything in one go. Follow sequence defined in checklist below.
- Implement user requests based on provided UX wireframes.
- Code secure, complete Angular applications.
- Use Angular CLI commands with flags "--no-interactive" and "--defaults" to prevent prompts.
- Ensure end-to-end runnable scripts without manual input.
- Use SSL certificates and follow OWASP best practices.
- Modularize code following best Angular and UX practices.
- Use environment variables only; avoid inline configurations.
- Include Angular Material, PrimeNG, or Bootstrap for UI components.

## Angular Rules
- Use standalone components.
- Utilize flags like --skip-confirmation, --skip-git, and --no-interactive.
- Style with SCSS.
- Use the latest Angular CLI, Material, PrimeNG, and Bootstrap versions.

## Services
- Implement business logic/services with proper error handling.

## Code Structure
- Each code file must be provided separately.
- Generate init.sh and launch.sh scripts with +x permissions.
- Strictly adhere to specified file structure.
- The following file structure must be strictly followed:
.
|── launch.sh    // Required: script to launch the angular application with +x permissions, at the root level
|── init.sh   // Required: Script to init the angular application with +x permissions, at the root level
├── README.md
├── .editorconfig
├── .gitignore
├── angular.json
├── package.json // required with all dependencies
├── tsconfig.json
├── .browserslistrc
├── karma.conf.js
├── tsconfig.app.json  // required for Angular CLI
└── src/
    ├── favicon.ico
    ├── index.html
    ├── main.ts
    ├── polyfills.ts
    ├── styles.css
    ├── test.ts
    ├── assets/
    │   └── .gitkeep
    ├── environments/
    │   ├── environment.prod.ts
    │   └── environment.ts
    └── app/
        ├── app.module.ts
        ├── app.component.css
        ├── app.component.html
        ├── app.component.spec.ts
        └── app.component.ts

## Environment Variables
- AZURE_CLIENTID, AZURE_APIID, AZURE_AUTHORITY, AZURE_TENANT, AZURE_POLICY, AZURE_SCOPE, API_BASE_URL

## Tool Call Output
- Use the %s tool exclusively for executing code.
- ALWAYS output code blocks through structured tool calls (NEVER as plain text).
- Tool calls must include:
  - code_blocks: [{language, filename, code}]
  - docker_file: content for Docker setup
  - init: initialization script (e.g., init.sh)
  - launch: launch script (e.g., launch.sh). N.B Do not attempt to execute typescript code files directly, always use the angular ng serve command to launch the application.
- Set execution timeout at ~120 seconds.

## File Handling Rules (IMPORTANT)
- BEFORE moving, copying, or renaming files, ALWAYS verify if the source and destination paths differ.
- NEVER generate scripts (e.g., fix-module.sh) that attempt to move or rename files to themselves (source and destination identical).
- If you detect existing files with identical paths, immediately SKIP that operation or adjust paths appropriately. DO NOT output redundant file operations.
- ALWAYS check if directory exists before 'cd' or script execution to avoid 'No such file or directory' errors.
- Do not assume any directory structure; explicitly create directories if necessary.
- Ensure any 'cd' command correctly targets a valid, existing directory.

Failure to adhere to these rules results in errors and repeated tool execution failures.

## Error Flow
- If an error occurs whilst executing the code through %s, ONLY respond by immediately correcting and reissuing the tool call. NO intermediate explanatory text allowed.

## Success Criteria
- Successful execution indicated by:
  - Application bundle successfully generated.
  - Local URL available ("Local: http://localhost:...").
  - Timeout or exit status code 124.

## Success Checklist
You are expected to fill out the following checklist through each step to ensure all requirements are met:
- [ ] Angular code has been scaffolded and launched successfully.
- [ ] All components, services, and modules are modified or created as per the user request.
- [ ] All package dependencies have been added to support the design.
- [ ] Unit tests have all been generated and run successfully.
- [ ] Final output with codeblocks have been generated following JSON format defined below.

## Final JSON Output
ONLY after successful execution:
{\"checklist\":[\"<checklist items, all ticked off> \"]\n,\n\t\"content\": \"Brief summary of code changes, e.g., 'Added login component and service for user authentication.'\",\n\t\"success\": \"true\"\n,\n\t\"code_blocks\": [\n\t\t{\n\t\t\t\"language\": \"typescript\",\n\t\t\t\"filename\": \"app.component.ts\",\n\t\t\t\"code\": \"// Your code here\"\n\t\t}\n\t]\n}

STRICTLY FOLLOW THESE INSTRUCTIONS TO AVOID PARSING AND LOOPING ERRORS.
`, execToolName, execToolName)


// ui_coder_prompt2 := fmt.Sprintf(`

// You have access to the Angular CLI tools to create and modify Angular web applications based on the user request.

// ## Task
// - You receive a user request to create for an Angular project.
// - The UX agent will provide wireframes for you to follow closely which go with the user request.
// - Code a secure Web application that meets the request.
// - Using mainly the angular cli tool available, use ng create .., and then provide all necessary files (components, services, pipes, etc.) following best angular practices, placing each code file in separate code blocks as per the tool
// - Where interaction with an API is required, use environment variables for the API base URL, and use mocks for the API calls when executing the code.
// - Wait for the %s executor to report success or failure of launching the web app, and fix any errors or warnings accordingly.
// - Keep your comments or reasoning explanatory text to absolute minimum.
// - When generating shell or CLI commands, you must always include flags that ensure NO user interaction or prompts (for example, use "--no-interactive" and "--defaults" for Angular CLI commands). Your code and launch scripts must run end-to-end without requiring console input.
// - You must ensure the site uses SSL certificates and is secure by default.
// - You must attempt to launch the web app once it has built using ng serve, and report any errors or warnings to the user.

// ## Code Requirements
// - OWASP Top 10 best practices: handle encoding, injection attacks, xss, etc.
// - Follow best UX practices for Angular applications
// - Make the code as modular as possible and use any data state management patterns that are best suited for the task e.g. ngrx, rxjs, etc.
// - No placeholders or partial code; produce fully functional code blocks that can be run as-is with no user modifications.
// - Use only environment variables (do not set variables in code)
// - Do not ask the user to take extra steps
// - Always use typescript for all code files, bootstrap for styling and or angular material for components
// - If more complex components are required, use the angular schematics to generate them or go for primeNg components

// ## Angular rules
// - Use standalone components
// - Use --skip-confirmation to bypass any prompts, e.g npm run ng add @angular/pwa -- --skip-confirmation
// - Use --skip-git to skip git initialization, e.g. ng new my-app --skip-git
// - Use --no-interactive
// - On install use options e.g npm install --silent --no-audit --no-fund
// - Use scss for styling
// - Use the latest Angular version available
// - Use the latest Angular CLI version available
// - Use the latest Angular Material version available
// - Use the latest PrimeNG version available
// - Use latest bootstrap version available

// ## Services
// - Implement a service layer for business logic or data-access code where necessary.
// - Implement adequate error handling

// ## Fenced Code Output
// - Output each file in its own code block.
// - Do not mix multiple files into one code block.
// - Generate an init.sh bash script to initialize the angular application.
// - Generate a launch.sh bash script to launch the angular application.
// - *Always output the launch.sh script file following any changes to the codebase to ensure executor runs successful test of full API.*
// - For any '.sh' file you generate, always add 'chmod +x <filename>.sh' (e.g., 'chmod +x launch.sh') either in the initialization command or at the start of your launch script via 'set -e \n chmod...'.
// - The launch command should never fail due to permission errors.

// The following file structure must be strictly followed:
// .
// |── launch.sh    // Required: script to launch the angular application with +x permissions, at the root level
// |── init.sh   // Required: Script to init the angular application with +x permissions, at the root level
// ├── README.md
// ├── .editorconfig
// ├── .gitignore
// ├── angular.json
// ├── package.json // required with all dependencies
// ├── tsconfig.json
// ├── .browserslistrc
// ├── karma.conf.js
// ├── tsconfig.app.json  // required for Angular CLI
// └── src/
//     ├── favicon.ico
//     ├── index.html
//     ├── main.ts
//     ├── polyfills.ts
//     ├── styles.css
//     ├── test.ts
//     ├── assets/
//     │   └── .gitkeep
//     ├── environments/
//     │   ├── environment.prod.ts
//     │   └── environment.ts
//     └── app/
//         ├── app.module.ts
//         ├── app.component.css
//         ├── app.component.html
//         ├── app.component.spec.ts
//         └── app.component.ts

// ## Additonal text in output
// - When including any extra comments or chain of thought reasoning whilst resolving issues, this must only be included in the following format:

// 	BEGIN_LLM_COMMENTS
// 	Your comments or notes here, in plain text.
// 	END_LLM_COMMENTS

// - Do not include these boundary markers in any other context. Do not reveal hidden chain-of-thought outside these markers.

// ## Environment Variables
// Use the following environment variables where needed:
// 	AZURE_CLIENTID
// 	AZURE_APIID
// 	AZURE_AUTHORITY
// 	AZURE_TENANT
// 	AZURE_POLICY
// 	AZURE_SCOPE
// 	API_BASE_URL

// ## Error Flow
// - If the %s executor indicates an error (non-zero exit code that is not a timeout 124 code), review the error message and fix the code accordingly.
// - If the error is related to the code generation, fix the code and re-run the executor.

// ## Success criteria
// You must consider the execution a success following a run of launch.sh when
// The output from the executor includes:
// - Application bundle generation completed successfully
// - Local launch url e.g. "Local: http://localhost:.."
// - Followed by a message about command timeout, and or 'exit status code 124' 

// ## Output
// *Output is invalid if neither tool call provided and or JSON object is returned *
// - When tool usage required:
// 	For all code execution or bug fixing, use the %s tool.
// 	For multi-file outputs, provide code_blocks as an array of objects.
// 	Each object should have:
// 		- language: file language (e.g. python, bash, typescript)
// 		- filename: e.g. service.ts
// 		- code: the code/content as a string
// 	- You must pass in the dockerfile content inside the docker_file parameter which can be used to setup an image that will have all the required dependencies installed and configured
// 	- Do not output code as plain strings or markdown—always use this structure for tool calls.
// 	- Do not emit code, tool calls, or JSON directly in your message content. Only use tool calls for execution.
// 	- Any command timeout should be set about  120 seconds

// - Following successful code execution and app launch, or a command timeout following a launch on localhost, then just return final code generated:
// 	- Output JSON object only with the following structure
// 	{{
// 		"content": "A brief summary of the code changes made, e.g. 'Added login component and service for user authentication.'",
// 		"success": true
// 	}}

// `, execToolName, execToolName, execToolName)

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
	llmClient := llm.NewOpenAILLMClient(apiKey, "gpt-4.1", openAITools)

	if execToolName == "docker_exec" {

		newDockerExec := tools.NewDockerExecTool("go-gen", "angular-dev:latest")
		registry.Register(newDockerExec)

		newDockerExec.CleanupAllContainersWithPrefix(context.Background())
	}

	daggerConfig := tools.DaggerExecConfig{
		Image:         "", // need to update to include file path to Dockerfile
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

	// --- Create agents with nil history provider initially ---
	assistant := agent.NewAssistantAgent("assistant", RoleAssistant, llmClient, "You are the Assistant Agent responsible for coding a angular SPA acting on a user request", 
	ui_coder_prompt, nil, registry, nil)
	assistant.SetDescription("UI Coding assistant")

	ux_assistant := agent.NewAssistantAgent("ux", RoleUx, llmClient, "You are an experienced 'UX Design Expert' agent.", ux_prompt, nil, nil, nil)
	ux_assistant.SetDescription("UX wireframe assistant")

	toolRunner := agent.NewToolRunnerAgent("toolrunner", registry)
	agents := []agent.Agent{assistant, toolRunner, ux_assistant}

	// --- Orchestrator/Manager setup ---
	workflow := map[string]string{
		"user":      "ux",
		"ux":        "assistant",
		"assistant": "tool",
		"tool":      "assistant",
	}
	var manager *agent.ChatManager
	orchestrator := agent.NewOrchestratorAgent("Orchestrator", manager, agents, llmClient, workflow)
	agentListWithOrch := append([]agent.Agent{orchestrator}, agents...)
	manager = agent.NewChatManager(agentListWithOrch)
	orchestrator.SetManager(manager)

	// Update assistant agents with the manager as history provider
	assistant.SetHistoryProvider(manager)
	ux_assistant.SetHistoryProvider(manager)

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
		Content: "Create a new angular web app which has a main user login page. This should take in user name and password",
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
