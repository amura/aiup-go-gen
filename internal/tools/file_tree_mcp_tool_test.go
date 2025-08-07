package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"aiupstart.com/go-gen/internal/common"
)

func TestFileTreeMcpTool_ListFiles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create file tree tool
	tool := NewFileTreeMcpTool()
	defer tool.Cleanup(ctx)

	// Test basic list files operation
	call := common.ToolCall{
		Name:   "file_tree",
		Caller: "test",
		Args: map[string]interface{}{
			"path": ".",
		},
	}

	result := tool.Call(ctx, call)

	// Check that we got some output
	if result.Error != nil {
		t.Errorf("Expected no error, got: %v", result.Error)
	}

	if result.Output == "" {
		t.Error("Expected non-empty output")
	}

	t.Logf("File tree output: %s", result.Output)
}

func TestFileTreeMcpTool_ListFilesWithOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create file tree tool
	tool := NewFileTreeMcpTool()
	defer tool.Cleanup(ctx)

	// Test with various options
	call := common.ToolCall{
		Name:   "file_tree",
		Caller: "test",
		Args: map[string]interface{}{
			"path":             ".",
			"max_depth":        float64(2),
			"show_hidden":      false,
			"directories_only": true,
			"exclude_pattern":  "node_modules",
		},
	}

	result := tool.Call(ctx, call)

	// Check that we got some output
	if result.Error != nil {
		t.Errorf("Expected no error, got: %v", result.Error)
	}

	if result.Output == "" {
		t.Error("Expected non-empty output")
	}

	t.Logf("File tree output with options: %s", result.Output)
}

func TestFileTreeMcpTool_BuildTreeCommand(t *testing.T) {
	tool := NewFileTreeMcpTool()

	// Test basic command
	args := map[string]interface{}{
		"path": "/test",
	}
	cmd := tool.buildTreeCommand("/test", args)
	expected := "cd /workspace && tree /test -I \"node_modules|.git|.env*|.DS_Store|*.log|.tmp|tmp|*.cache|dist|build|target|bin|obj|__pycache__|*.pyc\" 2>/dev/null"
	if cmd != expected {
		t.Errorf("Expected command %q, got %q", expected, cmd)
	}

	// Test with max depth
	args["max_depth"] = float64(3)
	cmd = tool.buildTreeCommand("/test", args)
	if !strings.Contains(cmd, "-L 3") {
		t.Error("Expected command to contain -L 3")
	}

	// Test with show hidden
	args["show_hidden"] = true
	cmd = tool.buildTreeCommand("/test", args)
	if !strings.Contains(cmd, "-a") {
		t.Error("Expected command to contain -a")
	}

	// Test with directories only
	args["directories_only"] = true
	cmd = tool.buildTreeCommand("/test", args)
	if !strings.Contains(cmd, "-d") {
		t.Error("Expected command to contain -d")
	}
}

func TestFileTreeMcpTool_BuildFallbackCommand(t *testing.T) {
	tool := NewFileTreeMcpTool()

	// Test basic fallback command
	args := map[string]interface{}{
		"path": "/test",
	}
	cmd := tool.buildFallbackCommand("/test", args)
	expected := "cd /workspace && find /test ! -path \"*/node_modules/*\" ! -path \"*/.git/*\" ! -path \"*/.env*\" ! -path \"*/.DS_Store\" ! -path \"*.log\" ! -path \"*/tmp/*\" ! -path \"*/.tmp/*\" ! -path \"*.cache\" ! -path \"*/dist/*\" ! -path \"*/build/*\" ! -path \"*/target/*\" ! -path \"*/bin/*\" ! -path \"*/obj/*\" ! -path \"*/__pycache__/*\" ! -path \"*.pyc\" | sort"
	if cmd != expected {
		t.Errorf("Expected fallback command %q, got %q", expected, cmd)
	}

	// Test with max depth
	args["max_depth"] = float64(2)
	cmd = tool.buildFallbackCommand("/test", args)
	if !strings.Contains(cmd, "-maxdepth 2") {
		t.Error("Expected fallback command to contain -maxdepth 2")
	}
}

func TestFileTreeMcpTool_FormatOutput(t *testing.T) {
	tool := NewFileTreeMcpTool()

	// Test basic formatting
	output := ".\n├── file1.txt\n└── dir1/\n    └── file2.txt"
	args := map[string]interface{}{
		"path": "/test",
	}
	formatted := tool.formatOutput(output, "/test", args)

	if !strings.Contains(formatted, "📁 File tree for path: /test") {
		t.Error("Expected formatted output to contain path header")
	}

	if !strings.Contains(formatted, output) {
		t.Error("Expected formatted output to contain original output")
	}

	// Test with options
	args["max_depth"] = float64(3)
	args["show_hidden"] = true
	formatted = tool.formatOutput(output, "/test", args)

	if !strings.Contains(formatted, "Options:") {
		t.Error("Expected formatted output to contain options")
	}
	if !strings.Contains(formatted, "max_depth=3") {
		t.Error("Expected formatted output to contain max_depth option")
	}
	if !strings.Contains(formatted, "show_hidden=true") {
		t.Error("Expected formatted output to contain show_hidden option")
	}
}
