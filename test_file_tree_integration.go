package test

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/config"
	"aiupstart.com/go-gen/internal/tools"
)

func main() {
	// Load the MCP configuration
	mcpCfgPath := filepath.Join(".", "mcp_tools.yaml")
	mcp_cfg, err := config.LoadConfig(mcpCfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create registry and register tools
	registry := tools.NewToolRegistry()

	for _, mcp := range mcp_cfg.McpTools {
		// Handle special local tools
		if mcp.Name == "file_tree" {
			fileTreeTool := tools.NewFileTreeMcpToolWithOperations(mcp.Operations)
			registry.Register(fileTreeTool)
			fmt.Printf("✅ Registered file_tree tool with %d operations\n", len(mcp.Operations))
		} else {
			tool := &tools.GenericMcpTool{
				NameStr:        mcp.Name,
				Endpoint:       mcp.Endpoint,
				DescriptionStr: mcp.Description,
			}
			registry.Register(tool)
			fmt.Printf("✅ Registered %s tool\n", mcp.Name)
		}
	}

	// Test the file tree tool
	fmt.Println("\n🔧 Testing file_tree tool...")

	call := common.ToolCall{
		Name:   "file_tree",
		Caller: "integration_test",
		Args: map[string]interface{}{
			"path":        ".",
			"max_depth":   float64(2),
			"show_hidden": false,
		},
	}

	ctx := context.Background()
	result := registry.CallTool(ctx, call)

	if result.Error != nil {
		fmt.Printf("❌ Tool call failed: %v\n", result.Error)
	} else {
		fmt.Printf("✅ Tool call succeeded!\n")
		output := ""
		if result.Output != nil {
			if str, ok := result.Output.(string); ok {
				output = str
			} else {
				output = fmt.Sprintf("%v", result.Output)
			}
		}
		fmt.Printf("📄 Output (first 500 chars):\n%s\n", truncate(output, 500))
	}

	fmt.Println("\n📋 Tool registry summary:")
	fmt.Printf("Available tools: %s\n", registry.DescribeTools())
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
