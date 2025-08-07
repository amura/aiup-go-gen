// internal/tools/file_tree_tool.go
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

type FileTreeTool struct {
	dockerExec *DockerExecTool // Reference to docker exec tool for container operations
}

func NewFileTreeTool() *FileTreeTool {
	return &FileTreeTool{
		dockerExec: NewDockerExecTool("file-tree", DefaultDockerImage, uuid.NewString(), os.TempDir()),
	}
}

func (t *FileTreeTool) Name() string {
	return "file_tree"
}

func (t *FileTreeTool) Description() string {
	return "List files and directories in a tree structure using the tree command. Useful for exploring project structure and file organization."
}

func (t *FileTreeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to list (default is current directory)",
				"default":     ".",
			},
			"max_depth": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum depth to traverse (default is unlimited)",
			},
			"show_hidden": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to show hidden files and directories (default is false)",
				"default":     false,
			},
			"directories_only": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to show only directories (default is false)",
				"default":     false,
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "File pattern to match (e.g., \"*.go\", \"*.js\")",
			},
			"exclude_pattern": map[string]interface{}{
				"type":        "string",
				"description": "Pattern to exclude from results (e.g., \"node_modules\")",
			},
			"file_size": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to show file sizes (default is false)",
				"default":     false,
			},
		},
		"required": []string{},
	}
}

func (t *FileTreeTool) Call(ctx context.Context, call common.ToolCall) common.ToolResult {
	utils.Logger.Debug().Str("tool", t.Name()).Msg("Executing file_tree tool call")

	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()

	// Parse parameters
	path := "."
	if p, ok := call.Args["path"].(string); ok && p != "" {
		path = p
	}

	// Build tree command based on parameters
	treeCmd := t.buildTreeCommand(path, call.Args)

	utils.Logger.Debug().Str("tool", t.Name()).Msgf("Executing tree command: %s", treeCmd)

	// Ensure we have a container to run the command in
	if err := t.dockerExec.ensureContainer(ctx, "bash", nil); err != nil {
		utils.Logger.Error().Str("tool", t.Name()).Msgf("Failed to ensure container: %v", err)
		return common.ToolResult{Error: fmt.Errorf("failed to ensure container: %w", err)}
	}

	// Execute the tree command
	output, err := t.dockerExec.execInContainer(ctx, treeCmd, 30*time.Second)
	if err != nil {
		// Fallback to find/ls if tree command fails
		fallbackCmd := t.buildFallbackCommand(path, call.Args)
		utils.Logger.Debug().Str("tool", t.Name()).Msgf("Tree command failed, trying fallback: %s", fallbackCmd)
		output, err = t.dockerExec.execInContainer(ctx, fallbackCmd, 30*time.Second)
		if err != nil {
			return common.ToolResult{
				Error:  fmt.Errorf("failed to list files: %w", err),
				Output: output,
			}
		}
	}

	return common.ToolResult{
		Output: output,
	}
}

func (t *FileTreeTool) buildTreeCommand(path string, args map[string]interface{}) string {
	cmd := fmt.Sprintf("tree %s", path)

	// Add options based on parameters
	if maxDepth, ok := args["max_depth"].(float64); ok && maxDepth > 0 {
		cmd += fmt.Sprintf(" -L %d", int(maxDepth))
	}

	if showHidden, ok := args["show_hidden"].(bool); ok && showHidden {
		cmd += " -a"
	}

	if directoriesOnly, ok := args["directories_only"].(bool); ok && directoriesOnly {
		cmd += " -d"
	}

	if fileSize, ok := args["file_size"].(bool); ok && fileSize {
		cmd += " -s"
	}

	// Handle patterns
	var excludePatterns []string

	// Add default exclusions
	defaultExclusions := []string{
		"node_modules", ".git", ".env*", ".DS_Store", "*.log",
		".tmp", "tmp", "*.cache", "dist", "build", "target",
		"bin", "obj", "__pycache__", "*.pyc",
	}
	excludePatterns = append(excludePatterns, defaultExclusions...)

	// Add user-specified exclusions
	if excludePattern, ok := args["exclude_pattern"].(string); ok && excludePattern != "" {
		excludePatterns = append(excludePatterns, excludePattern)
	}

	if len(excludePatterns) > 0 {
		cmd += fmt.Sprintf(" -I \"%s\"", strings.Join(excludePatterns, "|"))
	}

	// Handle include pattern
	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		cmd += fmt.Sprintf(" -P \"%s\"", pattern)
	}

	// Add error handling
	cmd += " 2>/dev/null || echo 'Tree command not available or failed'"

	return cmd
}

func (t *FileTreeTool) buildFallbackCommand(path string, args map[string]interface{}) string {
	// Use find as fallback
	cmd := fmt.Sprintf("find %s", path)

	// Add max depth
	if maxDepth, ok := args["max_depth"].(float64); ok && maxDepth > 0 {
		cmd += fmt.Sprintf(" -maxdepth %d", int(maxDepth))
	}

	// Add type filter for directories only
	if directoriesOnly, ok := args["directories_only"].(bool); ok && directoriesOnly {
		cmd += " -type d"
	}

	// Add pattern matching
	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		cmd += fmt.Sprintf(" -name \"%s\"", pattern)
	}

	// Add exclusions
	excludePatterns := []string{
		"*/node_modules/*", "*/.git/*", "*/.env*", "*/.DS_Store",
		"*.log", "*/tmp/*", "*/.tmp/*", "*.cache", "*/dist/*",
		"*/build/*", "*/target/*", "*/bin/*", "*/obj/*",
		"*/__pycache__/*", "*.pyc",
	}

	// Add user-specified exclusions
	if excludePattern, ok := args["exclude_pattern"].(string); ok && excludePattern != "" {
		excludePatterns = append(excludePatterns, fmt.Sprintf("*/%s/*", excludePattern))
	}

	for _, exclude := range excludePatterns {
		cmd += fmt.Sprintf(" ! -path \"%s\"", exclude)
	}

	// Sort output and format
	cmd += " | sort"

	// Add file size information if requested
	if fileSize, ok := args["file_size"].(bool); ok && fileSize {
		cmd = fmt.Sprintf("(%s) | xargs ls -lah", cmd)
	}

	return cmd
}

// Cleanup method to clean up docker resources
func (t *FileTreeTool) Cleanup(ctx context.Context) error {
	if t.dockerExec != nil {
		return t.dockerExec.CleanupContainer(ctx)
	}
	return nil
}
