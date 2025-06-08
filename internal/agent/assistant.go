// internal/agent/assistant.go
package agent

import (
	"fmt"
	"regexp"
	"strings"

	"aiupstart.com/go-gen/internal/llm"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/tools"
	"aiupstart.com/go-gen/internal/utils"
)

const promptTemplate = `
You are an expert AI coding assistant. Your persona: %s

You can answer coding questions, help with Python, Go, Angular, and use special tools for advanced tasks.

---------------------------------------------

You have access to the following tools:
%s

---------------------------------------------

When requested to generate some code, you must execute, test, or verify code, and always use the docker_exec tool. This tool allows you to run Python, Bash, Node.js, dotnet, or Angular CLI code inside a secure Docker container.
You must not ask the user to run code on their machine or provide instructions for running code. Instead, you should always use the docker_exec tool to execute code in a controlled environment.
If you need to execute code, output a JSON object that specifies the tool to call and the arguments required for execution.

When you need to use a function/tool, reply ONLY with a JSON code block (no narrative), using the JSON format:
{
  "tool": "docker_exec",
  "args": {
    "language": "<language name, e.g. python, bash, node, dotnet, angular>",
    "code_blocks": "<code to execute as a string>",
    "requirements": "<optional: dependencies or package list>",
    "env": { "<ENV_VAR>": "value" },
    "init": "<optional: The name of the initialization script or commands to run before executing code, such as npm i, pip install dotnet packages, etc.>",
    "launch": "<optional: path to a shell script to execute when launching the main code, e.g. 'start.sh'>",
    "dockerfile": "<Contents of dockerfile required to run the code>"
  }
}

Example:
{
  "tool": "docker_exec",
  "args": {
    "language": "python",
    "code_blocks": "print('Hello world!')"  // or multiple code blocks ring fenced with triple backticks,
    "init": "pip install requests",
    "launch": "start.sh",
  }
}

Never output code for execution/testing directly—**always** use the docker_exec tool and follow the output json above.

Otherwise, answer directly.

User request: %s
`

const assistantPromptTemplate = `
You are an expert AI coding assistant. Your persona: %s

You must ALWAYS output code in RING-FENCED code blocks using triple backticks (T_B_T), specifying the language.
You start with the language name, followed on the next line by filename, then on the next line and onwards the full code block.
When producing multi-file outputs, output each file as a separate code block with the filename as a comment at the top.

For example:
T_B_T python\n# filename: main.py\nprint("hello world")\nT_B_T

...
T_B_T bash
# filename: start.sh
    echo "Start"
T_B_T

Always include language, filename and content blocks with T_B_T

---------------------------------------------

You have access to the following tools:
%s

---------------------------------------------

When the user requests code generation or bug fixing, reply ONLY with such ring-fenced code blocks.

If you attempt code execution, use the docker_exec tool as a JSON tool call, and provide all code to execute in code blocks as above.

If the previous execution failed, analyze the error shown, fix the code and retry.

When you need to use a function/tool, reply ONLY with a JSON code block (no narrative), using the JSON format:
{
  "tool": "docker_exec",
  "args": {
 "language": "<language name, e.g. python, bash, node, dotnet, angular>",
    "code_blocks": "<code to execute as a string>",
    "requirements": "<optional: dependencies or package list>",
    "env": { "<ENV_VAR>": "value" },
    "init": "<optional: The name of the initialization script or commands to run before executing code, such as npm i, pip install dotnet packages, etc.>",
    "launch": "<optional: path to a shell script to execute when launching the main code, e.g. 'start.sh'>",
    "dockerfile": "<Contents of dockerfile required to run the code>"
  }
}

Example:
{
  "tool": "docker_exec",
  "args": {
    "language": "bash",
    "code_blocks": "T_B_T bash\n# filename init.sh\n print('Hello world!')...T_B_T\nT_B_T....."  // or multiple code blocks ring fenced with triple backticks,
    "init": "init.sh",
    "launch": "start.sh",
  }
}

------------------------------------

User request: %s
`

type AssistantAgent struct {
	name         string
	llmClient    llm.LLMClient
	persona      string
	toolRegistry *tools.ToolRegistry
}

func NewAssistantAgent(name string, llmClient llm.LLMClient, persona string, registry *tools.ToolRegistry) *AssistantAgent {
	return &AssistantAgent{
		name:         name,
		llmClient:    llmClient,
		toolRegistry: registry,
		persona:      persona,
		// toolsPrompt:  registry.DescribeTools(),
	}
}

func (a *AssistantAgent) Name() string { return a.name }

func (a *AssistantAgent) Start(input <-chan model.Message, output chan<- model.Message) {
	go func() {
		for msg := range input {
			metrics.AgentMessagesTotal.WithLabelValues(a.name).Inc()
			// Compose a prompt that includes both persona and available tools
			// prompt := a.promptTpl + "\n" + a.toolsPrompt + "\nUser: " + msg.Content
			prompt := fmt.Sprintf(
				strings.ReplaceAll(assistantPromptTemplate, "T_B_T", "```"),
				a.persona,
				a.toolRegistry.DescribeTools(),
				msg.Content,
			)

			utils.Logger.Debug().Str("prompt", prompt[:20]).Msg("Prompt going to LLM")
			llmResp, err := a.llmClient.Generate(prompt)
			if err != nil {
				output <- model.Message{Sender: a.name, Content: "[LLM ERROR] " + err.Error()}
				continue
			}

			// utils.Logger.Debug().Str("llm_response", llmResp.Content).Msg("LLM response received")
			// if err != nil {
			//     output <- model.Message{Sender: a.name, Content: "[LLM ERROR] " + err.Error()}
			//     continue
			// }
			// fmt.Printf("[LLM Response from %s]: %s\n", a.name, llmResp)
			// utils.Logger.Debug().Str(("llm_response"), llmResp.Content).Msg("About to parse response and check for tool calling")

			// // --- OpenAI function calling: check ToolCalls ---
			// if len(llmResp.ToolCalls) > 0 {
			//     for _, toolCall := range llmResp.ToolCalls {
			//         if a.toolRegistry.HasTool(toolCall.Name) {
			//             utils.Logger.Debug().
			//                 Str("tool_call", fmt.Sprintf("%+v", toolCall)).
			//                 Msg("Tool call from OpenAI response")
			//             output <- model.Message{
			//                 Sender:      a.name,
			//                 MessageType: model.TypeToolCall,
			//                 ToolCall: &tools.ToolCall{
			//                     Name:   toolCall.Name,
			//                     Args:   toolCall.Args,
			//                     Caller: a.name,
			//                 },
			//                 Content: fmt.Sprintf("Tool call for %s", toolCall.Name),
			//             }
			//         }
			//     }
			//     continue
			// }



            // --- OpenAI function calling: check ToolCalls ---
            if len(llmResp.ToolCalls) > 0 {
                for _, toolCall := range llmResp.ToolCalls {
                    if a.toolRegistry.HasTool(toolCall.Name) {
                        utils.Logger.Debug().
                            Str("tool_call", fmt.Sprintf("%+v", toolCall)).
                            Msg("Tool call from OpenAI response")
                        output <- model.Message{
                            Sender:      a.name,
                            MessageType: model.TypeToolCall,
                            ToolCall: &tools.ToolCall{
			                    Name:   toolCall.Name,
			                    Args:   toolCall.Args,
			                    Caller: a.name,
			                },
                        }
                    }
                }
                continue
            }

			// Try parsing as a tool suggestion
			toolCall, toolDetected := tools.ParseToolCall(llmResp.Content)
			if toolDetected && a.toolRegistry.HasTool(toolCall.Name) {
				utils.Logger.Debug().Str("tool_call", fmt.Sprintf("%+v", toolCall)).Msg("Tool call created from LLM response\n")

				// Instead of running, delegate to HITL agent by sending tool call message
				// output <- model.Message{Sender: a.name, Content: llmResp.Content, MessageType: model.TypeToolCall, ToolCall: &toolCall}
				output <- model.Message{
					Sender:      a.name,
					MessageType: model.TypeToolCall,
					ToolCall:    &toolCall,
				}
				continue

			}
			utils.Logger.Debug().Msg("No tool call detected in LLM response, sending direct response")
			// Otherwise, just output the LLM’s direct response
			// Otherwise, just output the LLM’s direct response
			output <- model.Message{
				Sender:      a.name,
				Content:     llmResp.Content,
				MessageType: model.TypeChat,
			}
		}
	}()
}

// Returns first JSON code block if present, else empty string
func ExtractFirstJsonBlock(s string) string {
	// Regex for ```json ... ```
	re := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	matches := re.FindStringSubmatch(s)
	if len(matches) >= 2 {
		return matches[1]
	}
	// Fallback: try to find any {...} JSON
	re2 := regexp.MustCompile("(?s)(\\{\\s*\"tool\"\\s*:\\s*\"[^\"]+\".*\\})")
	matches2 := re2.FindStringSubmatch(s)
	if len(matches2) >= 2 {
		return matches2[1]
	}
	return ""
}
