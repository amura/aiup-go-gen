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
	"time"

	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/google/uuid"
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
	Language string `json:"language"`
	FileName string `json:"filename"`
	Code     string `json:"code"`
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

// Should return the function schema for OpenAI, or a description of accepted parameters
func (t *DockerExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"language":   "string",
		"code_blocks": []map[string]string{
			{
				"language": "string",
				"filename": "string",
				"code":     "string",
			},
		},
		"init":   "string",
		"launch": "string",
	}
}

// Language to Docker image mapping
var langImageMap = map[string]string{
    "python":    "python:3.11-slim",
    "bash":      "angular-dev:latest", //"ubuntu:22.04", //todo later
    "sh":        "angular-dev:latest", //alpine:latest",
    "dotnet":    "mcr.microsoft.com/dotnet/sdk:8.0",
    "npm":       "node:20",
    // "angular":   "node:20", // angular cli must be npm-installed in init if needed
    "angular": "angular-dev:latest",
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
    utils.Logger.Debug().Str("containerName", containerName).Str("image", img).Msg("About to start the container for the session")

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

func (t *DockerExecTool) copyFilesToContainer(ctx context.Context, files []CodeBlock) error {
	utils.Logger.Debug().Str("tool", t.Name()).Msg("About to loop over files and write them out")
	for _, f := range files {
		dest := filepath.Join(t.workspace, f.FileName)
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("Processing file %s", dest)

		// Ensure the parent directories exist
		parentDir := filepath.Dir(dest)
        utils.Logger.Debug().Str("tool", t.Name()).Msgf("Checking parent path exists and creating if not")
		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			utils.Logger.Error().Str("tool", t.Name()).Msgf("Failed to create directories in path %s: %v", parentDir, err)
			return fmt.Errorf("error creating directories in path %s: %w", parentDir, err)
		}

		if err := os.WriteFile(dest, []byte(f.Code), 0644); err != nil {
			utils.Logger.Error().Str("tool", t.Name()).Msgf("Failed to write file: %s: %v", dest, err)
			return fmt.Errorf("failed to write temp file: %w", err)
		}
		// No need to docker cp since we mounted the workspace on start
	}
	return nil
}

func (t *DockerExecTool) execInContainer(ctx context.Context, command string, timeout time.Duration) (string, error) {
    args := []string{"exec", t.containerName, "sh", "-c", command}
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "docker", args...)
    output, err := cmd.CombinedOutput()

    if ctx.Err() == context.DeadlineExceeded {
        return string(output), fmt.Errorf("command timed out after %v", timeout)
    }
    return string(output), err
}

func (t *DockerExecTool) Call(ctx context.Context, call ToolCall) ToolResult {
	utils.Logger.Debug().Str("tool", t.Name()).Msgf("###############################\nExecuting docker_exec tool call: %v \n##############################", call.Caller)
	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()
    timeoutSec := 90 // default
    if to, ok := call.Args["timeout"].(float64); ok {
        timeoutSec = int(to)
    }
    timeoutDuration := time.Duration(timeoutSec)*time.Second

	langRaw, _ := call.Args["language"]
	lang := strings.ToLower(fmt.Sprintf("%v", langRaw))

	if err := t.ensureContainer(ctx, lang); err != nil {
		utils.Logger.Error().Msgf("Failed to ensure container: %v", err)
		return ToolResult{Error: err}
	}

    
	// 1. Parse code_blocks as array of CodeBlock
	rawBlocks, ok := call.Args["code_blocks"].([]interface{})
	if !ok || len(rawBlocks) == 0 {
		return ToolResult{Error: fmt.Errorf("missing or invalid code_blocks argument")}
	}
    utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to process code blocks - %v", len(rawBlocks))
	var blocks []CodeBlock
	for _, b := range rawBlocks {
		var cb CodeBlock
		// If map[string]interface{}, must re-marshal/re-unmarshal for type safety
		bytes, _ := utils.SafeMarshal(b)
		if err := utils.SafeUnmarshal(bytes, &cb); err == nil {
			blocks = append(blocks, cb)
		}
	}
	if len(blocks) == 0 {
		return ToolResult{Error: fmt.Errorf("no valid code blocks found")}
	}

	if err := t.copyFilesToContainer(ctx, blocks); err != nil {
		return ToolResult{Error: err}
	}

    utils.Logger.Debug().Str("tool", t.Name()).Msg("Finished copying files to container and about to run init and launch commands")

	// 2. Run "init" command, if any
	initCmd, _ := call.Args["init"].(string)
	if strings.TrimSpace(initCmd) != "" {
        utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute the init command %s", initCmd)
		initOut, err := t.execInContainer(ctx, initCmd, timeoutDuration)
		if err != nil {
            errDetail := formatExecError("init", initCmd, initOut, err.Error())
            return ToolResult{
                Output: errDetail,
                Error: err,
                ErrorDetail: &ExecErrorDetail{
                    Phase:   "init",
                    Command: initCmd,
                    Output:  initOut,
                    ErrMsg:  err.Error(),
                },
            }
		}
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("Init command output: %s", initOut)
	}

	// 3. Run launch or constructed main command
	launchCmd, _ := call.Args["launch"].(string)
	var output string
	var err error
	if strings.TrimSpace(launchCmd) != "" {
        // todo if angular, path the command to ensure no TTY expected
        launchCmd = patchAngularCmd(launchCmd)
        utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute launch command %s", launchCmd)
		output, err = t.execInContainer(ctx, launchCmd, timeoutDuration)
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
        utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute launch command %s", run)
		output, err = t.execInContainer(ctx, run, timeoutDuration)
	}

    if err != nil {
        errDetail := formatExecError("launch", launchCmd, output, err.Error())
        return ToolResult{
            Output: errDetail,
            Error:  fmt.Errorf("init failed: %w", err),
            ErrorDetail: &ExecErrorDetail{
                Phase:   "launch",
                Command: launchCmd,
                Output:  output,
                ErrMsg:  err.Error(),
            },
        }
    }
	return ToolResult{
		Output: output,
		Error:  err,
	}
}

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

func patchAngularCmd(cmd string) string {
	if strings.HasPrefix(cmd, "ng new") {
		if !strings.Contains(cmd, "--no-interactive") {
			cmd += " --no-interactive"
		}
		if !strings.Contains(cmd, "--defaults") {
			cmd += " --defaults"
		}
	}
	return cmd
}



func formatExecError(phase, cmd, output, errMsg string) string {
	return fmt.Sprintf(`
    [ERROR: Docker Exec - %s phase]
    Command:
    %s

    Output/Error:
    %s

    Internal error: %s
    `, strings.ToUpper(phase), cmd, output, errMsg)
}