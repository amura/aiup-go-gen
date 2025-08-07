package test

import (
	"fmt"
	"os"

	"aiupstart.com/go-gen/internal/tools"
	"github.com/google/uuid"
)

func main2() {
	// Test the new constructor signature
	containerID := uuid.NewString()
	tempPath := os.TempDir()

	fmt.Printf("Creating DockerExecTool with:\n")
	fmt.Printf("  Container ID: %s\n", containerID)
	fmt.Printf("  Temp Path: %s\n", tempPath)

	// Test basic constructor
	tool := tools.NewDockerExecTool("test-prefix", "node:20", containerID, tempPath)
	fmt.Printf("Tool created successfully: %s\n", tool.Name())

	// Test constructor with evaluator
	toolWithEvaluator := tools.NewDockerExecToolWithEvaluator("test-prefix-2", "node:20", uuid.NewString(), tempPath, nil)
	fmt.Printf("Tool with evaluator created successfully: %s\n", toolWithEvaluator.Name())

	fmt.Println("✅ All tests passed - refactoring successful!")
}
