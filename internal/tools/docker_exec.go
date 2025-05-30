// internal/tools/docker_exec.go
package tools

import (
    "context"
    "fmt"
    "os/exec"
    "strings"
	"aiupstart.com/go-gen/internal/utils"
)

type DockerExecTool struct{}

func (t *DockerExecTool) Name() string        { return "docker_exec" }
func (t *DockerExecTool) Description() string { return "Execute code/scripts in a Docker container. Supports python, bash, sh, dotnet, angular cli, npm." }


func (t *DockerExecTool) Parameters() map[string]string {
    return map[string]string{
        "language": "string",
        "code":     "string",
        "script":   "string", // alternative to code, if providing a shell script path
    }
}

// Language to Docker image mapping (could also be loaded from config)
var langImageMap = map[string]string{
    "python":    "python:3.11-slim",
    "bash":      "ubuntu:22.04",
    "sh":        "alpine:latest",
    "dotnet":    "mcr.microsoft.com/dotnet/sdk:8.0",
    "angular":   "node:20", // user must npm install @angular/cli in code block!
    "npm":       "node:20",
}

func (t *DockerExecTool) Call(ctx context.Context, call ToolCall) ToolResult {
    langRaw, ok := call.Args["language"]
    if !ok {
        return ToolResult{Error: fmt.Errorf("missing argument: language")}
    }
    lang := strings.ToLower(fmt.Sprintf("%v", langRaw))
    img, ok := langImageMap[lang]
    if !ok {
        return ToolResult{Error: fmt.Errorf("unsupported language: %s", lang)}
    }

    code, _ := call.Args["code"].(string)
    script, _ := call.Args["script"].(string)
    var content string
    var filename string
    var runCmd []string

    // Compose file/script depending on language
    switch lang {
    case "python":
        filename = "code.py"
        content = code
        runCmd = []string{"python", filename}
    case "bash", "sh":
        filename = "script.sh"
        if code != "" {
            content = code
        } else {
            content = script
        }
        runCmd = []string{"sh", filename}
    case "dotnet":
        filename = "Program.cs"
        content = code
        runCmd = []string{"dotnet", "run", "--project", filename}
    case "angular":
        filename = "app.js"
        content = code
        runCmd = []string{"npx", "ng", "run", filename}
    case "npm":
        filename = "script.js"
        content = code
        runCmd = []string{"node", filename}
    default:
        return ToolResult{Error: fmt.Errorf("unhandled language: %s", lang)}
    }

    // Write content to a temp file (host-side)
    tmpDir := utils.CreateTempDir()
    utils.WriteTempFile(tmpDir, filename, content)
    defer utils.RemoveTempDir(tmpDir)

    // Construct Docker run command
    dockerCmd := []string{
        "docker", "run", "--rm",
        "-v", fmt.Sprintf("%s:/workspace", tmpDir),
        "-w", "/workspace",
        img,
    }
    dockerCmd = append(dockerCmd, runCmd...)

    // Actually run the docker command
    output, err := exec.CommandContext(ctx, dockerCmd[0], dockerCmd[1:]...).CombinedOutput()
    return ToolResult{
        Output: string(output),
        Error:  err,
    }
}