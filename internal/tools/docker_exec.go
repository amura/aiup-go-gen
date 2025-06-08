// internal/tools/docker_exec.go
package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
    "github.com/google/uuid"

	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/prometheus/client_golang/prometheus"
)

type DockerExecTool struct{
    containerID   string
    image         string
    containerName string
    prefix        string
    workspace     string
	mu            sync.Mutex // for concurrency safety
}

type CodeBlock struct {
	Language string
	FileName string
	Content  string
}

const DefaultDockerImage = "node:20"

func NewDockerExecTool(prefix string, image string) *DockerExecTool {
    if image == "" {
        image = DefaultDockerImage
    }
    return &DockerExecTool{
        image:  image,
        prefix: prefix,
    }
}

func (t *DockerExecTool) Name() string        { return "docker_exec" }
func (t *DockerExecTool) Description() string { return "Execute code/scripts in a persistent Docker container. Supports python, bash, sh, dotnet, angular cli, npm." }


func (t *DockerExecTool) Parameters() map[string]string {
    return map[string]string{
        "language": "string",
        "code":     "string",
        "script":   "string", // alternative to code, if providing a shell script path
    }
}

// Language to Docker image mapping (could also be loaded from config)
var langImageMap = map[string]string{
    "python":    "python:3.11-slim",
    "bash":      "ubuntu:22.04",
    "sh":        "alpine:latest",
    "dotnet":    "mcr.microsoft.com/dotnet/sdk:8.0",
    "npm":       "node:20",
    "angular": "node:20", // assuming Angular CLI is installed in the container
}


// Ensure persistent container for the session
func (t *DockerExecTool) ensureContainer(ctx context.Context, lang string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.containerName != "" {
		// Optionally check "docker inspect" if you want to verify running
		return nil
	}
	id := uuid.NewString()
	img := t.image
	if limg, ok := langImageMap[lang]; ok {
		img = limg
	}
	containerName := fmt.Sprintf("%s-%s", t.prefix, id)
	workspace := filepath.Join(os.TempDir(), "dockerexec-"+id)
	os.MkdirAll(workspace, 0o755)
	// Run container detached
	cmd := exec.CommandContext(ctx, "docker", "run", "-d", "--name", containerName, "-w", "/workspace", "-v", workspace+":/workspace", img, "tail", "-f", "/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start container: %v - output: %s", err, string(out))
	}
	t.containerName = containerName
	t.workspace = workspace
	utils.Logger.Debug().Str("container", containerName).Msg("Started persistent Docker container")
	return nil
}

// Copy files to container (using docker cp)
func (t *DockerExecTool) copyFilesToContainer(ctx context.Context, files []CodeBlock) error {
	for _, f := range files {
		dest := filepath.Join(t.workspace, f.FileName)
		if err := os.WriteFile(dest, []byte(f.Content), 0644); err != nil {
			return fmt.Errorf("failed to write temp file: %v", err)
		}
		// No need to docker cp since we mounted the workspace on start
	}
	return nil
}

func (t *DockerExecTool) execInContainer(ctx context.Context, command string) (string, error) {
	args := []string{"exec", t.containerName, "sh", "-c", command}
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return string(output), err
}

// ExtractCodeBlocks parses an array of strings in code_blocks format
func ExtractCodeBlocks(blocks []string) []CodeBlock {
	var out []CodeBlock
	re := utils.CodeBlockRegex() // see below, or use your preferred regex
	for _, block := range blocks {
		match := re.FindStringSubmatch(block)
		if len(match) >= 4 {
			lang := strings.TrimSpace(match[1])
			filename := ""
			content := match[3]
			lines := strings.SplitN(match[2], "\n", 2)
			if len(lines) > 0 && strings.HasPrefix(lines[0], "# filename:") {
				filename = strings.TrimSpace(strings.TrimPrefix(lines[0], "# filename:"))
				if len(lines) > 1 {
					content = lines[1]
				}
			}
			if filename == "" {
				filename = "main.txt"
			}
			out = append(out, CodeBlock{
				Language: lang,
				FileName: filename,
				Content:  content,
			})
		}
	}
	return out
}

func (t *DockerExecTool) Call(ctx context.Context, call ToolCall) ToolResult {
	utils.Logger.Debug().Str("tool", t.Name()).Msgf("Executing docker_exec tool call: %v", call.Caller)
	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()
	langRaw, _ := call.Args["language"]
	lang := strings.ToLower(fmt.Sprintf("%v", langRaw))

	if err := t.ensureContainer(ctx, lang); err != nil {
		utils.Logger.Error().Msgf("Failed to ensure container: %v", err)
		return ToolResult{Error: err}
	}

	// 1. Extract all code blocks and write to workspace
	rawBlocks, ok := call.Args["code_blocks"].([]interface{})
	if !ok || len(rawBlocks) == 0 {
		return ToolResult{Error: fmt.Errorf("missing or invalid code_blocks argument")}
	}
	var codeBlockStrs []string
	for _, b := range rawBlocks {
		if s, ok := b.(string); ok {
			codeBlockStrs = append(codeBlockStrs, s)
		}
	}
	blocks := ExtractCodeBlocks(codeBlockStrs)
	if err := t.copyFilesToContainer(ctx, blocks); err != nil {
		return ToolResult{Error: err}
	}

	// 2. Run "init" command, if any
	initCmd, _ := call.Args["init"].(string)
	if strings.TrimSpace(initCmd) != "" {
		initOut, err := t.execInContainer(ctx, initCmd)
		if err != nil {
			return ToolResult{Output: initOut, Error: fmt.Errorf("init failed: %w", err)}
		}
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("Init command output: %s", initOut)
	}

	// 3. Run launch or constructed main command
	launchCmd, _ := call.Args["launch"].(string)
	var output string
	var err error
	if strings.TrimSpace(launchCmd) != "" {
		output, err = t.execInContainer(ctx, launchCmd)
	} else if len(blocks) > 0 {
		mainfile := blocks[0].FileName
		run := ""
		switch lang {
		case "python":
			run = fmt.Sprintf("python %s", mainfile)
		case "bash", "sh":
			run = fmt.Sprintf("sh %s", mainfile)
		case "dotnet":
			run = fmt.Sprintf("dotnet run --project %s", mainfile)
		case "npm", "angular":
			run = fmt.Sprintf("npm start")
		default:
			run = fmt.Sprintf("sh %s", mainfile)
		}
		output, err = t.execInContainer(ctx, run)
	}

	return ToolResult{
		Output: output,
		Error:  err,
	}
}

// func (t *DockerExecTool) Call(ctx context.Context, call ToolCall) ToolResult {
//     fmt.Print("Executing docker_exec tool call: ", call.Caller, "\n")
//     metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
// 	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
// 	defer timer.ObserveDuration()
// 	langRaw, _ := call.Args["language"]
// 	lang := strings.ToLower(fmt.Sprintf("%v", langRaw))

// 	if err := t.ensureContainer(ctx, lang); err != nil {
// 		utils.Logger.Error().Msgf("Failed to ensure container: %v", err)
// 		return ToolResult{Error: err}
// 	}

// 	// 1. Extract all code blocks and write to workspace
// 	code, _ := call.Args["code"].(string)
// 	blocks := utils.ExtractCodeBlocks(code)
// 	if err := t.copyFilesToContainer(ctx, blocks); err != nil {
// 		return ToolResult{Error: err}
// 	}

// 	// 2. Run "init" command, if any
// 	initCmd, _ := call.Args["init"].(string)
// 	if strings.TrimSpace(initCmd) != "" {
// 		initOut, err := t.execInContainer(ctx, initCmd)
// 		if err != nil {
// 			return ToolResult{Output: initOut, Error: fmt.Errorf("init failed: %w", err)}
// 		}
//         utils.Logger.Debug().Str("tool", t.Name()).
//             Msgf("Init command output: %s", initOut)
// 	}

// 	// 3. Run launch or constructed main command
// 	launchCmd, _ := call.Args["launch"].(string)
// 	var output string
// 	var err error
// 	if strings.TrimSpace(launchCmd) != "" {
// 		output, err = t.execInContainer(ctx, launchCmd)
// 	} else {
// 		// For demo: just run the first file if launch not specified
// 		if len(blocks) > 0 {
// 			mainfile := blocks[0].FileName
// 			run := ""
// 			switch lang {
// 			case "python":
// 				run = fmt.Sprintf("python %s", mainfile)
// 			case "bash", "sh":
// 				run = fmt.Sprintf("sh %s", mainfile)
// 			case "dotnet":
// 				run = fmt.Sprintf("dotnet run --project %s", mainfile)
// 			case "npm", "angular":
// 				run = fmt.Sprintf("npm start")
// 			default:
// 				run = fmt.Sprintf("sh %s", mainfile)
// 			}
// 			output, err = t.execInContainer(ctx, run)
// 		}
// 	}

// 	return ToolResult{
// 		Output: output,
// 		Error:  err,
// 	}
// }

// Clean up (call at session end or from manager)
func (t *DockerExecTool) CleanupContainer(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.containerName != "" {
		exec.CommandContext(ctx, "docker", "rm", "-f", t.containerName).Run()
		utils.Logger.Debug().Str("container", t.containerName).Msg("Cleaned up Docker container")
		t.containerName = ""
	}
	if t.workspace != "" {
		os.RemoveAll(t.workspace)
	}
	return nil
}

// Static: Clean up all by prefix (e.g. crash recovery)
func CleanupAllByPrefix(ctx context.Context, prefix string) {
	cmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf(`docker ps -a --filter "name=%s" -q | xargs docker rm -f`, prefix))
	cmd.Run()
}

// func (t *DockerExecTool) Call(ctx context.Context, call ToolCall) ToolResult {
//     fmt.Print("Executing docker_exec tool call: ", call.Caller, "\n")
//     metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
// 	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
// 	defer timer.ObserveDuration()

//     utils.Logger.Debug().Str("tool", t.Name()).
//         Str("caller", call.Caller).
//         Msgf("Received call with args: %v", call.Args)

//     langRaw, ok := call.Args["language"]
//     if !ok {
//         return ToolResult{Error: fmt.Errorf("missing argument: language")}
//     }
//     lang := strings.ToLower(fmt.Sprintf("%v", langRaw))
//     img, ok := langImageMap[lang]
//     if !ok {
//         return ToolResult{Error: fmt.Errorf("unsupported language: %s", lang)}
//     }
//     code, _ := call.Args["code"].(string)
//     initCmd, _ := call.Args["init"].(string)
//     launchCmd, _ := call.Args["launch"].(string)

//     // Write all code blocks to temp files
//     tmpDir := utils.CreateTempDir()
//     blocks := utils.ExtractCodeBlocks(code)
//     filenames := []string{}
//     for i, block := range blocks {
//         ext := lang
//         if block.Language != "" {
//             ext = block.Language
//         }
//         fname := fmt.Sprintf("file%d.%s", i+1, ext)
//         utils.WriteTempFile(tmpDir, fname, block.Content)
//         utils.Logger.Debug().Str("tool", t.Name()).
//             Msgf("Wrote code block to temp file: %s", fname)
//         filenames = append(filenames, fname)
//     }
//     defer utils.RemoveTempDir(tmpDir)

//     // Build docker base command
//     dockerBase := []string{
//         "docker", "run", "--rm",
//         "-v", fmt.Sprintf("%s:/workspace", tmpDir),
//         "-w", "/workspace",
//         img,
//     }

//     // Step 1: run init, if present
//     if strings.TrimSpace(initCmd) != "" {
//         utils.Logger.Debug().Str("tool", t.Name()).
//             Msgf("Running init command: %s", initCmd)
//         cmd := append(dockerBase, "sh", "-c", initCmd)
//         output, err := exec.CommandContext(ctx, cmd[0], cmd[1:]...).CombinedOutput()
//         if err != nil {
//             utils.Logger.Error().
//                 Str("tool", t.Name()).
//                 Msgf("Init command failed: %v, output: %s", err, string(output))
//             return ToolResult{Output: string(output), Error: fmt.Errorf("init failed: %w", err)}
//         }
//     }

//     // Step 2: run launch, or constructed command
//     var cmd []string
//     if strings.TrimSpace(launchCmd) != "" {
//         utils.Logger.Debug().Str("tool", t.Name()).
//             Msgf("Running launch command: %s", launchCmd)
//         cmd = append(dockerBase, "sh", "-c", launchCmd)
//     } else {
//         utils.Logger.Debug().Str("tool", t.Name()).
//             Msgf("No launch command provided, constructing from language: %s", lang)
//         // Pick the first file as the "main" file for this language
//         fname := ""
//         if len(filenames) > 0 {
//             fname = filenames[0]
//         }
//         switch lang {
//         case "python":
//             cmd = append(dockerBase, "python", fname)
//         case "bash", "sh":
//             cmd = append(dockerBase, "sh", fname)
//         case "dotnet":
//             cmd = append(dockerBase, "dotnet", "run", "--project", fname)
//         case "npm", "angular":
//             cmd = append(dockerBase, "sh", "-c", fmt.Sprintf("npm install && %s", fname))
//         default:
//             cmd = append(dockerBase, "sh", fname)
//         }
//     }
//     utils.Logger.Debug().Str("tool", t.Name()).Msgf("Running command: %v", cmd)
//     output, err := exec.CommandContext(ctx, cmd[0], cmd[1:]...).CombinedOutput()
//     if err != nil {
//         utils.Logger.Error().
//             Str("tool", t.Name()).
//             Msgf("Launch command failed: %v, output: %s", err, string(output))
//         return ToolResult{Output: string(output), Error: fmt.Errorf("launch failed: %w", err)}
//     }
//     return ToolResult{Output: string(output)}
// }