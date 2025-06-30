// Example usage of DockerExecTool with custom error evaluator
package main

import (
	"context"
	"strings"

	"aiupstart.com/go-gen/internal/tools"
)

// Example error evaluator function that considers Angular timeout errors as acceptable
func angularTimeoutEvaluator(errDetail, phase, command, output, errMsg string) bool {
	// Consider Angular serve timeouts as successful since the server likely started
	if phase == "launch" && strings.Contains(command, "ng serve") && strings.Contains(errMsg, "timed out") {
		// Check if the output indicates successful startup
		if strings.Contains(output, "webpack compiled") ||
			strings.Contains(output, "Local:") ||
			strings.Contains(output, "served at") {
			return true // Consider this a pass
		}
	}

	// Consider npm install warnings as acceptable in init phase
	if phase == "init" && strings.Contains(command, "npm install") {
		// Only fail on actual errors, not warnings
		if strings.Contains(output, "WARN") && !strings.Contains(output, "ERR") {
			return true // Consider warnings as pass
		}
	}

	return false // Default to failure for all other cases
}

// Example of creating DockerExecTool with custom error evaluator
func createAngularDockerTool() *tools.DockerExecTool {
	return tools.NewDockerExecToolWithEvaluator(
		"angular-test",          // prefix
		"angular-dev:latest",    // image
		angularTimeoutEvaluator, // error evaluator function
	)
}

// Example of creating DockerExecTool without error evaluator (default behavior)
func createDefaultDockerTool() *tools.DockerExecTool {
	return tools.NewDockerExecTool(
		"default-test", // prefix
		"node:20",      // image
	)
}

func main() {
	ctx := context.Background()

	// Create tool with custom error evaluator
	tool := createAngularDockerTool()

	// Use the tool...
	_ = tool
	_ = ctx
}
