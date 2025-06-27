package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiupstart.com/go-gen/internal/common"
)

func TestDaggerExecTool_BasicNodeRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Prepare a temp dir for output (ensuring clean-up)
	outDir, err := os.MkdirTemp("", "dagger-test-output-*")
	if err != nil {
		t.Fatalf("failed to create temp output dir: %v", err)
	}
	defer os.RemoveAll(outDir) // cleanup always

	// Example: Node script that writes to a file
	const jsCode = `
const fs = require('fs');
fs.writeFileSync('output.txt', 'Hello from Dagger!');
console.log('Done.');
`

	// 1. Prepare CodeBlock
	blocks := []CodeBlock{
		{
			FileName: "test.js",
			Language: "javascript",
			Code:     jsCode,
		},
	}

	// Write code blocks to outDir
	for _, b := range blocks {
		filePath := filepath.Join(outDir, b.FileName)
		if err := os.WriteFile(filePath, []byte(b.Code), 0644); err != nil {
			t.Fatalf("failed to write code block to file: %v", err)
		}
	}

	// Convert blocks to []map[string]interface{}
	var codeBlocksArg []map[string]interface{}
	for _, b := range blocks {
		codeBlocksArg = append(codeBlocksArg, map[string]interface{}{
			"file_name": b.FileName,
			"language":  b.Language,
			"code":      b.Code,
		})
	}

	// Convert to []interface{} for compatibility with tool.Run
	var codeBlocksIface []interface{}
	for _, cb := range codeBlocksArg {
		codeBlocksIface = append(codeBlocksIface, cb)
	}

	// 2. Tool config
	cfg := DaggerExecConfig{
		Image:          "node:20",
		Workdir:        "/src",
		MountPath:      "/src",
		OutputPath:     "/src", // expects output.txt here
		Timeout:        60 * time.Second,
		CopyToHostDir:  outDir,
		DockerfilePath: "/Users/amakura/source/repos/aiup-go-gen/dockerfiles", // not needed for this test
	}

	tool := NewDaggerExecTool("dagger_exec", "Regression test", cfg)

	// 3. ToolCall input
	call := &common.ToolCall{
		Caller: "TestCase",
		Args: map[string]interface{}{
			"code_blocks": codeBlocksIface,
			"launch":      "node test.js",
		},
	}

	// 4. Execute!
	result, err := tool.Run(ctx, call)
	if err != nil {
		t.Fatalf("tool run error: %v; output: %s", err, result.Output)
	}

	// 5. Assert output text
	if !strings.Contains(result.Output.(string), "Done.") {
		t.Errorf("expected script output in result, got: %s", result.Output)
	}

	// 6. Assert file is copied to outDir
	outFile := filepath.Join(outDir, "output.txt")
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Errorf("expected output.txt to exist, err: %v", err)
	}
	if string(data) != "Hello from Dagger!" {
		t.Errorf("unexpected file content: %q", data)
	}

	// 7. Cleanup handled by defer
}
