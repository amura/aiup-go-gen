// internal/tools/file_tree_mcp_tool.go
package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/config"
	"aiupstart.com/go-gen/internal/metrics"
	"aiupstart.com/go-gen/internal/utils"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
)

// FileTreeMcpTool implements the MCP-style tool interface for file tree operations
type FileTreeMcpTool struct {
	dockerExec *DockerExecTool           // Reference to docker exec tool for container operations
	operations []config.McpToolOperation // Store the operations from configuration
}

func NewFileTreeMcpTool() *FileTreeMcpTool {
	return &FileTreeMcpTool{
		dockerExec: NewDockerExecTool("file-tree", DefaultDockerImage, uuid.NewString(), os.TempDir()),
	}
}

func NewFileTreeMcpToolWithOperations(operations []config.McpToolOperation) *FileTreeMcpTool {
	return &FileTreeMcpTool{
		dockerExec: NewDockerExecTool("file-tree", DefaultDockerImage, uuid.NewString(), os.TempDir()),
		operations: operations,
	}
}

func (t *FileTreeMcpTool) Name() string {
	return "file_tree"
}

func (t *FileTreeMcpTool) Description() string {
	return "List files and directories in a tree structure using the tree command. Useful for exploring project structure and file organization."
}

func (t *FileTreeMcpTool) Parameters() map[string]interface{} {
	// If we have operations from config, use the first operation's parameters
	if len(t.operations) > 0 {
		op := t.operations[0] // Use list_files operation
		if op.Parameters != nil {
			return op.Parameters
		}
	}

	// Fallback to default parameters
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

func (t *FileTreeMcpTool) Call(ctx context.Context, call common.ToolCall) common.ToolResult {
	utils.Logger.Debug().Str("tool", t.Name()).Msg("Executing file_tree MCP tool call")

	metrics.ToolCallsTotal.WithLabelValues(t.Name(), call.Caller).Inc()
	timer := prometheus.NewTimer(metrics.ToolLatencySeconds.WithLabelValues(t.Name(), call.Caller))
	defer timer.ObserveDuration()

	// The operation is implicit since we only have list_files operation for this tool
	// But we could check for operation name if multiple operations are supported
	return t.handleListFiles(ctx, call)
}

func (t *FileTreeMcpTool) handleListFiles(ctx context.Context, call common.ToolCall) common.ToolResult {
	// Parse parameters with defaults
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
	if err != nil || strings.Contains(output, "command not found") || strings.Contains(output, "tree: not found") {
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

	// Format the output
	formattedOutput := t.formatOutput(output, path, call.Args)

	return common.ToolResult{
		Output: formattedOutput,
	}
}

func (t *FileTreeMcpTool) buildTreeCommand(path string, args map[string]interface{}) string {
	cmd := fmt.Sprintf("cd /workspace && tree %s", path)

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
	cmd += " 2>/dev/null"

	return cmd
}

func (t *FileTreeMcpTool) buildFallbackCommand(path string, args map[string]interface{}) string {
	// Use find as fallback
	cmd := fmt.Sprintf("cd /workspace && find %s", path)

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
		cmd = fmt.Sprintf("(%s) | xargs ls -lah 2>/dev/null", cmd)
	}

	return cmd
}

func (t *FileTreeMcpTool) formatOutput(output, path string, args map[string]interface{}) string {
	if strings.TrimSpace(output) == "" {
		return fmt.Sprintf("No files found in path: %s", path)
	}

	result := fmt.Sprintf("📁 File tree for path: %s\n\n", path)

	// Add parameter summary
	var options []string
	if maxDepth, ok := args["max_depth"].(float64); ok && maxDepth > 0 {
		options = append(options, fmt.Sprintf("max_depth=%d", int(maxDepth)))
	}
	if showHidden, ok := args["show_hidden"].(bool); ok && showHidden {
		options = append(options, "show_hidden=true")
	}
	if directoriesOnly, ok := args["directories_only"].(bool); ok && directoriesOnly {
		options = append(options, "directories_only=true")
	}
	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		options = append(options, fmt.Sprintf("pattern=%s", pattern))
	}
	if excludePattern, ok := args["exclude_pattern"].(string); ok && excludePattern != "" {
		options = append(options, fmt.Sprintf("exclude_pattern=%s", excludePattern))
	}
	if fileSize, ok := args["file_size"].(bool); ok && fileSize {
		options = append(options, "file_size=true")
	}

	if len(options) > 0 {
		result += fmt.Sprintf("Options: %s\n\n", strings.Join(options, ", "))
	}

	result += output

	return result
}

// Cleanup method to clean up docker resources
func (t *FileTreeMcpTool) Cleanup(ctx context.Context) error {
	if t.dockerExec != nil {
		return t.dockerExec.CleanupContainer(ctx)
	}
	return nil
}
