package aiup_go_gen

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
)

// CodeResult represents the result of executing a code block.
type CodeResult struct {
    Output   string // Standard output and error combined
    ExitCode int    // Exit code of the execution
    Error    error  // Error encountered during execution, if any
}

// CodeExecutor defines the interface for executing code blocks.
type CodeExecutor interface {
    ExecuteCodeBlock(block CodeBlock) CodeResult
}

// NoOpFileWriterExecutor writes code blocks to files without executing them.
type NoOpFileWriterExecutor struct {
    WorkDir string // Directory where code files will be written
}

// ExecuteCodeBlock writes the code block to a file in the specified working directory.
func (e NoOpFileWriterExecutor) ExecuteCodeBlock(block CodeBlock) CodeResult {
    absPath := filepath.Join(e.WorkDir, block.Filename)
    dir := filepath.Dir(absPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to create directory: %w", err),
        }
    }
    err := os.WriteFile(absPath, []byte(block.Code), 0644)
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to write file: %w", err),
        }
    }
    return CodeResult{
        Output:   fmt.Sprintf("Code written to %s", absPath),
        ExitCode: 0,
        Error:    nil,
    }
}

// ShellCommandExecutor executes code blocks as shell scripts.
type ShellCommandExecutor struct {
    WorkDir string // Directory where code files will be written and executed
}

// ExecuteCodeBlock writes the code block to a file and executes it as a shell script.
func (e ShellCommandExecutor) ExecuteCodeBlock(block CodeBlock) CodeResult {
    absPath := filepath.Join(e.WorkDir, block.Filename)
    dir := filepath.Dir(absPath)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to create directory: %w", err),
        }
    }
    err := os.WriteFile(absPath, []byte(block.Code), 0755)
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to write file: %w", err),
        }
    }

    cmd := exec.Command("sh", absPath)
    cmd.Dir = e.WorkDir
    output, err := cmd.CombinedOutput()
    exitCode := 0
    if err != nil {
        if exitError, ok := err.(*exec.ExitError); ok {
            exitCode = exitError.ExitCode()
        } else {
            exitCode = 1
        }
    }

    return CodeResult{
        Output:   string(output),
        ExitCode: exitCode,
        Error:    err,
    }
}