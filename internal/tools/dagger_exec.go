package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/model"
	"aiupstart.com/go-gen/internal/utils"
	"dagger.io/dagger"
	"dagger.io/dagger/dag"
	"github.com/prometheus/client_golang/prometheus"
)

type DaggerExecConfig struct {
	Image         string            // e.g., "node:20"
	Workdir       string            // e.g., "/src"
	MountPath     string            // e.g., "/src"
	OutputPath    string            // e.g., "/src/dist"
	Timeout       time.Duration     // e.g., 3 * time.Minute
	Env           map[string]string // e.g., {"API_BASE_URL": "..."}
	CopyToHostDir string            // e.g., "./dist" (optional)
	DockerfilePath string // e.g., "./Dockerfile" (optional, for local builds)
}

type DaggerExecTool struct {
	name        string
	description string
	config      DaggerExecConfig
}

// Parameters implements Tool.
func (t *DaggerExecTool) Parameters() map[string]interface{} {
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

func NewDaggerExecTool(name, description string, config DaggerExecConfig) *DaggerExecTool {
	return &DaggerExecTool{name: name, description: description, config: config}
}

func (t *DaggerExecTool) Name() string        { return t.name }
func (t *DaggerExecTool) Description() string { return t.description }

func (t *DaggerExecTool) Run(ctx context.Context, call *common.ToolCall) (*common.ToolResult, error) {
	utils.Logger.Debug().
		Str("tool", t.Name()).
		Msgf("###############################\nExecuting dagger_exec tool call: %v \n##############################", call.Caller)

	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()

	// Step 1: Setup Dagger client
	client, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stdout))
	if err != nil {
		utils.Logger.Error().Err(err).Msg("Failed to connect to Dagger")
		return &common.ToolResult{ErrorMsg: err.Error()}, err
	}
	defer client.Close()

	// 1. Parse code_blocks as array of CodeBlock
	rawBlocks, ok := call.Args["code_blocks"].([]interface{})
	if !ok || len(rawBlocks) == 0 {
		err := fmt.Errorf("missing or invalid code_blocks argument")
		return &common.ToolResult{Role: model.RoleTool, ErrorMsg: err.Error()}, err
	}
	utils.Logger.Debug().Str("tool", t.Name()).Msgf("About to process code blocks - %v", len(rawBlocks))
	var blocks []CodeBlock
	for _, b := range rawBlocks {
		var cb CodeBlock
		bytes, _ := utils.SafeMarshal(b)
		if err := utils.SafeUnmarshal(bytes, &cb); err == nil {
			blocks = append(blocks, cb)
		}
	}
	if len(blocks) == 0 {
		err := fmt.Errorf("no valid code blocks found")
		return &common.ToolResult{Role: model.RoleTool, ErrorMsg: err.Error()}, err
	}

	// Prepare source directory (in-memory or local)
	srcDir := client.Host().Directory(".", dagger.HostDirectoryOpts{Include: []string{"*"}}) // fallback, override below

	// (Optional) For code_blocks content, write to temp dir (not shown here, but possible for full in-memory use)
	// See Dagger docs for full in-memory files usage.

	// Step 3: Set up container
	// Use a local image if the image name starts with "local://"
	absDockerfilesPath, err := filepath.Abs(t.config.DockerfilePath)
	if err != nil {
		utils.Logger.Error().Err(err).Msgf("Failed to resolve dockerfiles absolute path: %s", "./dockerfiles")
		panic(err) // or handle as needed
	}

	// (Optional: Confirm directory exists)
	if stat, err := os.Stat(absDockerfilesPath); err != nil || !stat.IsDir() {
		utils.Logger.Error().Err(err).Msgf("Dockerfiles path does not exist or is not a directory: %s", absDockerfilesPath)
		panic(err) // or handle as needed
	}

	srcDir = dag.Host().Directory(absDockerfilesPath)
	var container *dagger.Container

	if t.config.Image == "" || strings.HasPrefix(t.config.Image, "local://") {
		// Always build the image locally from Dockerfile
		container = client.Container().Build(srcDir).
			WithWorkdir(t.config.Workdir).
			WithMountedDirectory(t.config.MountPath, srcDir)
	} else {
		// Use a published image (from a registry)
		container = client.Container().From(t.config.Image).
			WithWorkdir(t.config.Workdir).
			WithMountedDirectory(t.config.MountPath, srcDir)
	}

	// Set environment variables (from config and call.Args["env"])
	for k, v := range t.config.Env {
		container = container.WithEnvVariable(k, v)
	}
	if envRaw, ok := call.Args["env"].(map[string]interface{}); ok {
		for k, v := range envRaw {
			if valStr, ok := v.(string); ok {
				container = container.WithEnvVariable(k, valStr)
			}
		}
	}

	// Step 4: Init/install step
	if initCmd, ok := call.Args["init"].(string); ok && initCmd != "" {
		utils.Logger.Info().Str("tool", t.Name()).Msgf("Running init command: %s", initCmd)
		container = container.WithExec(strings.Fields(initCmd))
	}

	// Step 5: Launch/main command
	launchCmd := "npm run start"
	if lcmd, ok := call.Args["launch"].(string); ok && lcmd != "" {
		launchCmd = lcmd
	}
	utils.Logger.Info().Str("tool", t.Name()).Msgf("Running launch command: %s", launchCmd)
	out, err := container.
		WithExec(strings.Fields(launchCmd), dagger.ContainerWithExecOpts{ExperimentalPrivilegedNesting: true}).
		Stdout(ctx)
	if err != nil {
		utils.Logger.Error().Err(err).Msg("Launch failed")
		return &common.ToolResult{
			Output:   fmt.Sprintf("Launch error: %v", err),
			ErrorMsg: err.Error(),
		}, err
	}

	// Step 6: Optionally copy output from container to host
	var copied string
	if t.config.CopyToHostDir != "" {
		outputDir := container.Directory(t.config.OutputPath)
		_, err := outputDir.Export(ctx, t.config.CopyToHostDir)
		if err != nil {
			utils.Logger.Error().Err(err).Msg("Failed to export output dir from container")
			return &common.ToolResult{
				Output:   fmt.Sprintf("Export error: %v", err),
				ErrorMsg: err.Error(),
			}, err
		}
		copied = fmt.Sprintf("Exported output to host: %s", t.config.CopyToHostDir)
	}

	utils.Logger.Info().Str("tool", t.Name()).Msgf("Launch output: %s", out)
	return &common.ToolResult{
		Output: fmt.Sprintf("Command output: %s\n%s", out, copied),
	}, nil
}

func (t *DaggerExecTool) Call(ctx context.Context, call common.ToolCall) common.ToolResult {
	utils.Logger.Debug().
		Str("tool", t.Name()).
		Msgf("###############################\nExecuting dagger_exec tool call: %v \n##############################", call.Caller)

	// Call the Run method
	result, err := t.Run(ctx, &call)
	if err != nil {
		utils.Logger.Error().Err(err).Msg("Tool execution failed")
		initCmd, _ := call.Args["init"].(string)
		return common.ToolResult{Role: model.RoleTool, 
			Output:     err.Error(),
				ErrorMsg:   err.Error(),
				ErrorPhase: "init",
				Command:    initCmd,
				CmdOutput:  "",
		
		}
	}

	return *result
}


