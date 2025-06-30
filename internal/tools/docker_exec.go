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

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// ErrorEvaluatorFunc is a function type that takes error details and determines
// if the execution should be considered a pass (true) or failure (false)
type ErrorEvaluatorFunc func(errDetail string, phase string, command string, output string, errMsg string) bool

type DockerExecTool struct {
	containerID    string
	image          string
	containerName  string
	prefix         string
	workspace      string
	errorEvaluator ErrorEvaluatorFunc // Function to evaluate if errors should be considered pass/fail
	mu             sync.Mutex         // for concurrency safety
}

type CodeBlock struct {
	Language string `json:"language"`
	FileName string `json:"filename"`
	Code     string `json:"code"`
}

const DefaultDockerImage = "node:20"

func NewDockerExecTool(prefix string, image string) *DockerExecTool {
	return NewDockerExecToolWithEvaluator(prefix, image, nil)
}

func NewDockerExecToolWithEvaluator(prefix string, image string, errorEvaluator ErrorEvaluatorFunc) *DockerExecTool {
	if image == "" {
		image = DefaultDockerImage
	}
	return &DockerExecTool{
		image:          image,
		prefix:         prefix,
		errorEvaluator: errorEvaluator,
	}
}

func (t *DockerExecTool) Name() string { return "docker_exec" }
func (t *DockerExecTool) Description() string {
	return "Execute code/scripts in a persistent Docker container. Supports python, bash, sh, dotnet, angular cli, npm."
}

// Should return the function schema for OpenAI, or a description of accepted parameters
func (t *DockerExecTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"language": "string",
		"code_blocks": []map[string]string{
			{
				"language": "string",
				"filename": "string",
				"code":     "string",
			},
		},
		"init":   "string",
		"launch": "string",
		"ports":  []string{"string"}, // e.g. ["8080:80"]
	}
}

// Language to Docker image mapping
var langImageMap = map[string]string{
	"python": "python:3.11-slim",
	"bash":   "angular-dev:latest", //"ubuntu:22.04", //todo later
	"sh":     "angular-dev:latest", //alpine:latest",
	"dotnet": "mcr.microsoft.com/dotnet/sdk:8.0",
	"npm":    "node:20",
	// "angular":   "node:20", // angular cli must be npm-installed in init if needed
	"angular": "angular-dev:latest",
}

// Ensure persistent container for the session
func (t *DockerExecTool) ensureContainer(ctx context.Context, lang string, ports []string) error {
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
	// Build docker run command with port mappings
	args := []string{"run", "-d", "--name", containerName, "-w", "/workspace", "-v", workspace + ":/workspace"}

	// Add port mappings
	if ports != nil {
		for _, port := range ports {
			args = append(args, "-p", fmt.Sprintf("%s", port))
		}
	}

	args = append(args, img, "tail", "-f", "/dev/null")

	utils.Logger.Debug().Str("containerName", containerName).Msgf("🚀 Running Docker command: docker %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "docker", args...)
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
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("Processing file %s", f.FileName)

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

	utils.Logger.Debug().Str("tool", t.Name()).Msg("Command has been started, next waiting for the output")

	// Use pipes to capture output in real-time
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Channel to collect output as it arrives
	outputChan := make(chan []byte, 100)
	var outputBuffer strings.Builder

	// Read stdout in goroutine
	go func() {
		defer close(outputChan)
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case outputChan <- chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Read stderr in goroutine
	stderrChan := make(chan []byte, 100)
	go func() {
		defer close(stderrChan)
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case stderrChan <- chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				break
			}
		}
	}()

	// Collect output from both channels until timeout or completion
	var cmdErr error
	cmdDone := make(chan error, 1)
	go func() {
		cmdDone <- cmd.Wait()
	}()

	for {
		select {
		case chunk, ok := <-outputChan:
			if ok {
				outputBuffer.Write(chunk)
				// Log partial output for debugging
				utils.Logger.Debug().Str("tool", t.Name()).Msgf("Stdout chunk: %s", string(chunk))
			}
		case chunk, ok := <-stderrChan:
			if ok {
				outputBuffer.Write(chunk)
				// Log partial output for debugging
				utils.Logger.Debug().Str("tool", t.Name()).Msgf("Stderr chunk: %s", string(chunk))
			}
		case cmdErr = <-cmdDone:
			// Command completed, collect any remaining output
			for {
				select {
				case chunk, ok := <-outputChan:
					if ok {
						outputBuffer.Write(chunk)
					} else {
						outputChan = nil
					}
				case chunk, ok := <-stderrChan:
					if ok {
						outputBuffer.Write(chunk)
					} else {
						stderrChan = nil
					}
				default:
					if outputChan == nil && stderrChan == nil {
						goto done
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
		done:
			finalOutput := outputBuffer.String()
			utils.Logger.Debug().Str("tool", t.Name()).Msgf("Command completed with output length: %d", len(finalOutput))
			return finalOutput, cmdErr
		case <-ctx.Done():
			// Timeout occurred - collect whatever output we have so far
			partialOutput := outputBuffer.String()
			utils.Logger.Debug().Str("tool", t.Name()).Msgf("Command timed out after %v, collected %d bytes of output", timeout, len(partialOutput))

			// Forcefully kill the process
			if cmd.Process != nil {
				utils.Logger.Debug().Str("tool", t.Name()).Msg("Killing process due to timeout")
				cmd.Process.Kill()
			}
			// Also try to kill the docker container process
			killCmd := exec.Command("docker", "exec", t.containerName, "pkill", "-f", command)
			killCmd.Run()

			// Return the partial output we captured before timeout
			if partialOutput != "" {
				return partialOutput, fmt.Errorf("command timed out after %v (partial output captured)", timeout)
			}
			return "", fmt.Errorf("command timed out after %v", timeout)
		}
	}
}

func (t *DockerExecTool) Run(ctx context.Context, call *common.ToolCall) (*common.ToolResult, error) {
	utils.Logger.Debug().
		Str("tool", t.Name()).
		Msgf("###############################\nExecuting docker_exec tool call: %v \n##############################", call.Caller)
	return &common.ToolResult{}, nil

	// metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	// timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	// defer timer.ObserveDuration()

	// timeoutSec := 90 // default
	// if to, ok := call.Args["timeout"].(float64); ok {
	// 	timeoutSec = int(to)
	// }
	// timeoutDuration := time.Duration(timeoutSec) * time.Second

	// langRaw, _ := call.Args["language"]
	// lang := strings.ToLower(fmt.Sprintf("%v", langRaw))

	// if err := t.ensureContainer(ctx, lang); err != nil {
	// 	utils.Logger.Error().Msgf("Failed to ensure container: %v", err)
	// 	return &common.ToolResult{
	// 		Role:     model.RoleTool,
	// 		ErrorMsg: fmt.Sprintf("Failed to ensure container: %v", err),
	// 	}, err
	// }

	// // 1. Parse code_blocks as array of CodeBlock
	// rawBlocks, ok := call.Args["code_blocks"].([]interface{})
	// if !ok || len(rawBlocks) == 0 {
	// 	err := fmt.Errorf("missing or invalid code_blocks argument")
	// 	return &common.ToolResult{Role: model.RoleTool, ErrorMsg: err.Error()}, err
	// }
	// utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to process code blocks - %v", len(rawBlocks))
	// var blocks []CodeBlock
	// for _, b := range rawBlocks {
	// 	var cb CodeBlock
	// 	bytes, _ := utils.SafeMarshal(b)
	// 	if err := utils.SafeUnmarshal(bytes, &cb); err == nil {
	// 		blocks = append(blocks, cb)
	// 	}
	// }
	// if len(blocks) == 0 {
	// 	err := fmt.Errorf("no valid code blocks found")
	// 	return &common.ToolResult{Role: model.RoleTool, ErrorMsg: err.Error()}, err
	// }

	// if err := t.copyFilesToContainer(ctx, blocks); err != nil {
	// 	utils.Logger.Error().Msgf("Failed to copy files: %v", err)
	// 	return &common.ToolResult{Role: model.RoleTool, ErrorMsg: err.Error()}, err
	// }

	// utils.Logger.Debug().Str("tool", t.Name()).Msg("Finished copying files to container and about to run init and launch commands")

	// // 2. Run "init" command, if any
	// initCmd, _ := call.Args["init"].(string)
	// if strings.TrimSpace(initCmd) != "" {
	// 	utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute the init command %s", initCmd)
	// 	initOut, err := t.execInContainer(ctx, initCmd, timeoutDuration)
	// 	if err != nil {
	// 		errDetail := formatExecError("init", initCmd, initOut, err.Error())
	// 		return &common.ToolResult{
	// 			Role:       model.RoleTool,
	// 			Output:     errDetail,
	// 			ErrorMsg:   err.Error(),
	// 			ErrorPhase: "init",
	// 			Command:    initCmd,
	// 			CmdOutput:  initOut,
	// 		}, err
	// 	}
	// 	utils.Logger.Debug().Str("tool", t.Name()).Msgf("Init command output: %s", initOut)
	// }

	// // 3. Run launch or constructed main command
	// launchCmd, _ := call.Args["launch"].(string)
	// var output string
	// var err error
	// if strings.TrimSpace(launchCmd) != "" {
	// 	launchCmd = patchAngularCmd(launchCmd)
	// 	utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute launch command %s", launchCmd)
	// 	output, err = t.execInContainer(ctx, launchCmd, timeoutDuration)
	// } else if len(blocks) > 0 {
	// 	mainfile := blocks[0].FileName
	// 	run := ""
	// 	switch lang {
	// 	case "python":
	// 		run = fmt.Sprintf("python %s", mainfile)
	// 	case "bash", "sh":
	// 		run = fmt.Sprintf("sh %s", mainfile)
	// 	case "dotnet":
	// 		run = fmt.Sprintf("dotnet run --project %s", mainfile)
	// 	case "npm", "angular":
	// 		run = fmt.Sprintf("npm start")
	// 	default:
	// 		run = fmt.Sprintf("sh %s", mainfile)
	// 	}
	// 	utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute launch command %s", run)
	// 	output, err = t.execInContainer(ctx, run, timeoutDuration)
	// }

	// if err != nil {
	// 	errDetail := formatExecError("launch", launchCmd, output, err.Error())
	// 	return &common.ToolResult{
	// 		Role:       model.RoleTool,
	// 		Output:     errDetail,
	// 		ErrorMsg:   err.Error(),
	// 		ErrorPhase: "launch",
	// 		Command:    launchCmd,
	// 		CmdOutput:  output,
	// 	}, err
	// }

	// return &common.ToolResult{
	// 	Role:   model.RoleTool,
	// 	Output: output,
	// }, nil
}

func (t *DockerExecTool) Call(ctx context.Context, call common.ToolCall) common.ToolResult {
	utils.Logger.Debug().Str("tool", t.Name()).Msgf("###############################\nExecuting docker_exec tool call: %v \n##############################", call.Caller)
	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()
	timeoutSec := 30 // default
	if to, ok := call.Args["timeout"].(float64); ok {
		timeoutSec = int(to)
	}
	timeoutDuration := time.Duration(timeoutSec) * time.Second
	utils.Logger.Debug().Str("tool", t.Name()).Msgf("⏱️  Configured timeout: %v (%d seconds)", timeoutDuration, timeoutSec)

	langRaw, _ := call.Args["language"]
	lang := strings.ToLower(fmt.Sprintf("%v", langRaw))

	// Parse ports from Args
	var ports []string
	if portsRaw, ok := call.Args["ports"].([]interface{}); ok {
		for _, p := range portsRaw {
			if portStr := fmt.Sprintf("%v", p); portStr != "" {
				ports = append(ports, portStr)
			}
		}
	}

	if err := t.ensureContainer(ctx, lang, ports); err != nil {
		utils.Logger.Error().Msgf("Failed to ensure container: %v", err)
		return common.ToolResult{Error: err}
	}

	// 1. Parse code_blocks as array of CodeBlock
	rawBlocks, ok := call.Args["code_blocks"].([]interface{})
	if !ok || len(rawBlocks) == 0 {
		return common.ToolResult{Error: fmt.Errorf("missing or invalid code_blocks argument")}
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
		return common.ToolResult{Error: fmt.Errorf("no valid code blocks found")}
	}

	if err := t.copyFilesToContainer(ctx, blocks); err != nil {
		return common.ToolResult{Error: err}
	}

	utils.Logger.Debug().Str("tool", t.Name()).Msg("Finished copying files to container and about to run init and launch commands")

	// 2. Run "init" command, if any
	initCmd, _ := call.Args["init"].(string)
	if strings.TrimSpace(initCmd) != "" {
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute the init command '%s' with timeout %v", initCmd, timeoutDuration)
		initOut, err := t.execInContainer(ctx, initCmd, timeoutDuration)
		if err != nil {
			// Get file listing for debugging
			fileList := t.getContainerFileList(ctx)
			// Get Angular error logs if available
			angularLogs := t.getAngularErrorLogs(ctx, initOut)
			errDetail := formatExecError("init", initCmd, initOut, err.Error())
			if fileList != "" {
				errDetail += "\n\n[DEBUG: Container filesystem contents]\n" + fileList
			}
			if angularLogs != "" {
				errDetail += "\n\n[DEBUG: Angular error logs]\n" + angularLogs
			}

			// Check if this error should be considered a pass or failure
			if t.evaluateError(errDetail, "init", initCmd, initOut, err.Error()) {
				// Error evaluator says this should be considered a pass
				utils.Logger.Debug().Str("tool", t.Name()).Msgf("Init command completed with acceptable error: %s", err.Error())
				return common.ToolResult{
					Output: errDetail,
					Error:  nil, // Clear the error to indicate pass
				}
			}

			return common.ToolResult{
				Output: errDetail,
				Error:  err,
				ErrorDetail: &common.ExecErrorDetail{
					Phase:   "init",
					Command: initCmd,
					Output:  initOut,
					ErrMsg:  err.Error(),
				},
			}
		}
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("Init command output: %s", initOut)
	} else {
		utils.Logger.Debug().Str("tool", t.Name()).Msg("🛑 No init command provided, skipping init phase")
	}

	// 3. Run launch or constructed main command
	launchCmd, _ := call.Args["launch"].(string)
	var output string
	var err error
	if strings.TrimSpace(launchCmd) != "" {
		// todo if angular, path the command to ensure no TTY expected
		launchCmd = patchAngularCmd(launchCmd)
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute launch command '%s' with timeout %v", launchCmd, timeoutDuration)
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
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to execute launch command '%s' with timeout %v", run, timeoutDuration)
		output, err = t.execInContainer(ctx, run, timeoutDuration)
	}

	if err != nil {
		// Get file listing for debugging
		fileList := t.getContainerFileList(ctx)
		// Get Angular error logs if available
		angularLogs := t.getAngularErrorLogs(ctx, output)
		errDetail := formatExecError("launch", launchCmd, output, err.Error())
		if fileList != "" {
			errDetail += "\n\n[DEBUG: Container filesystem contents]\n" + fileList
		}
		if angularLogs != "" {
			errDetail += "\n\n[DEBUG: Angular error logs]\n" + angularLogs
		}

		// Check if this error should be considered a pass or failure using the configured evaluator
		if t.evaluateError(errDetail, "launch", launchCmd, output, err.Error()) {
			utils.Logger.Warn().Str("tool", t.Name()).Msgf("Launch command evaluated as pass: %s", err.Error())
			return common.ToolResult{
				Output: errDetail,
				Error:  fmt.Errorf("launch succeeded with warnings: %w", err),
				ErrorDetail: &common.ExecErrorDetail{
					Phase:   "launch",
					Command: launchCmd,
					Output:  output,
					ErrMsg:  err.Error(),
				},
			}
		}

		utils.Logger.Error().Str("tool", t.Name()).Msgf("Launch command failed: %s", err.Error())
		return common.ToolResult{
			Output: errDetail,
			Error:  fmt.Errorf("launch failed: %w", err),
			ErrorDetail: &common.ExecErrorDetail{
				Phase:   "launch",
				Command: launchCmd,
				Output:  output,
				ErrMsg:  err.Error(),
			},
		}
	}
	return common.ToolResult{
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

// CleanupAllContainersWithPrefix removes all containers that match this instance's prefix pattern
func (t *DockerExecTool) CleanupAllContainersWithPrefix(ctx context.Context) error {
	if t.prefix == "" {
		return fmt.Errorf("prefix is empty, cannot cleanup containers")
	}

	utils.Logger.Debug().Str("prefix", t.prefix).Msg("Cleaning up all containers with prefix")

	// First, get all container IDs that match the prefix pattern
	listCmd := exec.CommandContext(ctx, "docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s-", t.prefix), "-q")
	output, err := listCmd.CombinedOutput()
	if err != nil {
		utils.Logger.Error().Str("prefix", t.prefix).Msgf("Failed to list containers with prefix: %v", err)
		return fmt.Errorf("failed to list containers with prefix %s: %w", t.prefix, err)
	}

	containerIDs := strings.Fields(strings.TrimSpace(string(output)))
	if len(containerIDs) == 0 {
		utils.Logger.Debug().Str("prefix", t.prefix).Msg("No containers found with prefix")
		return nil
	}

	// Remove all containers in one command
	args := append([]string{"rm", "-f"}, containerIDs...)
	removeCmd := exec.CommandContext(ctx, "docker", args...)
	if err := removeCmd.Run(); err != nil {
		utils.Logger.Error().Str("prefix", t.prefix).Msgf("Failed to remove containers: %v", err)
		return fmt.Errorf("failed to remove containers with prefix %s: %w", t.prefix, err)
	}

	utils.Logger.Debug().Str("prefix", t.prefix).Int("count", len(containerIDs)).Msg("Successfully cleaned up containers with prefix")
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
	// Handle ng serve and other potentially hanging commands
	if strings.HasPrefix(cmd, "ng serve") || strings.Contains(cmd, "ng serve") {
		// Add timeout to ng serve to prevent indefinite hanging
		cmd = fmt.Sprintf("timeout 30s %s", cmd)
	}
	return cmd
}

func formatExecError(phase, cmd, output, errMsg string) string {
	// Check if this is a timeout error and format accordingly
	isTimeout := strings.Contains(errMsg, "timed out")
	hasPartialOutput := strings.Contains(errMsg, "partial output captured")

	var statusMsg string
	if isTimeout {
		if hasPartialOutput {
			statusMsg = "⏱️  COMMAND TIMED OUT (Partial output captured before timeout)"
		} else {
			statusMsg = "⏱️  COMMAND TIMED OUT (No output captured)"
		}
	} else {
		statusMsg = "❌ COMMAND FAILED"
	}

	var outputSection string
	if output != "" {
		if isTimeout && hasPartialOutput {
			outputSection = fmt.Sprintf(`
    📤 Partial Output Captured Before Timeout:
    %s
    
    ⚠️  Note: Command was killed due to timeout - if the application was launched on localhost take that as a successful launch, else the above output may be incomplete`, output)
		} else {
			outputSection = fmt.Sprintf(`
    📤 Output/Error:
    %s`, output)
		}
	} else {
		if isTimeout {
			outputSection = `
    📤 Output/Error:
    (No output captured - command may have been stuck or running silently)`
		} else {
			outputSection = `
    📤 Output/Error:
    (No output)`
		}
	}

	return fmt.Sprintf(`
    [%s: Docker Exec - %s phase]
    🔧 Command:
    %s
%s

    🔍 Internal error: %s
    `, statusMsg, strings.ToUpper(phase), cmd, outputSection, errMsg)
}

// getContainerFileList returns a listing of files in the container for debugging
func (t *DockerExecTool) getContainerFileList(ctx context.Context) string {
	if t.containerName == "" {
		return "Container not available"
	}

	// First try tree command with exclusions
	listCmd := `tree /workspace -I "node_modules|.git|.env*|.DS_Store|*.log|.tmp|tmp|*.cache|dist|build|target|bin|obj|__pycache__|*.pyc" -a -L 4 2>/dev/null`
	output, err := t.execInContainer(ctx, listCmd, 10*time.Second)
	if err != nil || strings.Contains(output, "command not found") || strings.Contains(output, "tree: not found") {
		// Fallback to find with ls and exclusions
		listCmd = `find /workspace -type f ! -path "*/node_modules/*" ! -path "*/.git/*" ! -name ".env*" ! -name ".DS_Store" ! -name "*.log" ! -path "*/tmp/*" ! -path "*/.tmp/*" ! -name "*.cache" ! -path "*/dist/*" ! -path "*/build/*" ! -path "*/target/*" ! -path "*/bin/*" ! -path "*/obj/*" ! -path "*/__pycache__/*" ! -name "*.pyc" -exec ls -la {} \; 2>/dev/null | head -100`
		output, err = t.execInContainer(ctx, listCmd, 10*time.Second)
		if err != nil {
			// Final fallback to simple listing
			listCmd = "ls -la /workspace 2>/dev/null || echo 'No workspace files found'"
			output, _ = t.execInContainer(ctx, listCmd, 5*time.Second)
		}
	}

	if strings.TrimSpace(output) == "" {
		return "No files found in container workspace"
	}

	return output
}

// getAngularErrorLogs extracts Angular error log files from the container
func (t *DockerExecTool) getAngularErrorLogs(ctx context.Context, commandOutput string) string {
	if t.containerName == "" {
		return "Container not available"
	}

	var logFiles []string

	// Parse the output to find angular error log files mentioned in the error
	lines := strings.Split(commandOutput, "\n")
	for _, line := range lines {
		if strings.Contains(line, "angular-errors.log") {
			// Extract file path using regex or string parsing
			if strings.Contains(line, "See \"") && strings.Contains(line, "\" for further details") {
				start := strings.Index(line, "See \"") + 5
				end := strings.Index(line[start:], "\" for further details")
				if end > 0 {
					logFile := line[start : start+end]
					logFiles = append(logFiles, logFile)
				}
			}
		}
	}

	// Also check common Angular temp directories for error logs
	commonPaths := []string{
		"/tmp/ng-*/angular-errors.log",
		"/workspace/.angular/cache/angular-errors.log",
		"/workspace/angular-errors.log",
	}

	var allLogs strings.Builder
	allLogs.WriteString("Angular Error Logs:\n")

	// Get logs from paths found in output
	for _, logFile := range logFiles {
		allLogs.WriteString(fmt.Sprintf("\n=== Contents of %s ===\n", logFile))
		catCmd := fmt.Sprintf("cat %s 2>/dev/null || echo 'File not found or not readable'", logFile)
		logContent, err := t.execInContainer(ctx, catCmd, 5*time.Second)
		if err != nil {
			allLogs.WriteString(fmt.Sprintf("Error reading file: %v\n", err))
		} else {
			allLogs.WriteString(logContent)
		}
		allLogs.WriteString("\n")
	}

	// Search for common Angular error log locations
	for _, pattern := range commonPaths {
		findCmd := fmt.Sprintf("find %s -maxdepth 1 -type f 2>/dev/null | head -5", strings.Replace(pattern, "*", "", -1))
		// For glob patterns, use a different approach
		if strings.Contains(pattern, "*") {
			// Extract directory pattern and search
			dir := strings.Split(pattern, "*")[0]
			findCmd = fmt.Sprintf("find %s -name 'angular-errors.log' -type f 2>/dev/null | head -5", dir)
		}

		foundFiles, err := t.execInContainer(ctx, findCmd, 5*time.Second)
		if err == nil && strings.TrimSpace(foundFiles) != "" {
			files := strings.Split(strings.TrimSpace(foundFiles), "\n")
			for _, file := range files {
				if strings.TrimSpace(file) != "" && !contains(logFiles, file) {
					allLogs.WriteString(fmt.Sprintf("\n=== Contents of %s ===\n", file))
					catCmd := fmt.Sprintf("cat %s 2>/dev/null || echo 'File not found or not readable'", file)
					logContent, err := t.execInContainer(ctx, catCmd, 5*time.Second)
					if err != nil {
						allLogs.WriteString(fmt.Sprintf("Error reading file: %v\n", err))
					} else {
						allLogs.WriteString(logContent)
					}
					allLogs.WriteString("\n")
				}
			}
		}
	}

	// Also check for any ng-* directories in /tmp
	findTmpCmd := "find /tmp -name 'ng-*' -type d 2>/dev/null | head -10"
	tmpDirs, err := t.execInContainer(ctx, findTmpCmd, 5*time.Second)
	if err == nil && strings.TrimSpace(tmpDirs) != "" {
		dirs := strings.Split(strings.TrimSpace(tmpDirs), "\n")
		for _, dir := range dirs {
			if strings.TrimSpace(dir) != "" {
				logFile := filepath.Join(dir, "angular-errors.log")
				allLogs.WriteString(fmt.Sprintf("\n=== Contents of %s ===\n", logFile))
				catCmd := fmt.Sprintf("cat %s 2>/dev/null || echo 'File not found'", logFile)
				logContent, err := t.execInContainer(ctx, catCmd, 5*time.Second)
				if err != nil {
					allLogs.WriteString(fmt.Sprintf("Error reading file: %v\n", err))
				} else if !strings.Contains(logContent, "File not found") {
					allLogs.WriteString(logContent)
				}
				allLogs.WriteString("\n")
			}
		}
	}

	result := allLogs.String()
	if strings.TrimSpace(result) == "Angular Error Logs:" {
		return "No Angular error logs found"
	}

	return result
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// evaluateError uses the configured error evaluator function to determine
// if an error should be considered a pass (true) or failure (false)
func (t *DockerExecTool) evaluateError(errDetail, phase, command, output, errMsg string) bool {
	if t.errorEvaluator == nil {
		// No evaluator configured, treat all errors as failures
		return false
	}
	return t.errorEvaluator(errDetail, phase, command, output, errMsg)
}
