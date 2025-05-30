package aiup_go_gen
import (
    "bytes"
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/mount"
    "github.com/docker/docker/client"
    "github.com/docker/docker/pkg/stdcopy"
)

// DockerExecutor executes code blocks inside Docker containers.
type DockerExecutor struct {
    Image   string            // Docker image to use
    WorkDir string            // Host working directory
    Env     map[string]string // Environment variables to set in the container
    Shell   string            // Shell to use inside the container (e.g., "sh", "bash", "dotnet", "npm", "ng")
}

// ExecuteCodeBlock writes the code to a file, creates a Docker container, and executes the code inside it.
func (e DockerExecutor) ExecuteCodeBlock(block CodeBlock) CodeResult {
    ctx := context.Background()

    // Initialize Docker client
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to create Docker client: %w", err),
        }
    }
    defer cli.Close()

    // Write code to a temporary file
    hostFilePath := filepath.Join(e.WorkDir, block.Filename)
    if err := os.MkdirAll(filepath.Dir(hostFilePath), 0755); err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to create directory: %w", err),
        }
    }
    if err := os.WriteFile(hostFilePath, []byte(block.Code), 0644); err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to write code file: %w", err),
        }
    }

    // Prepare environment variables
    envVars := []string{}
    for key, value := range e.Env {
        envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
    }

    // Determine the command to execute based on the specified shell
    var cmd []string
    containerFilePath := filepath.Join("/workspace", block.Filename)
    switch e.Shell {
    case "sh", "bash":
        cmd = []string{e.Shell, "-c", fmt.Sprintf("chmod +x %s && %s", containerFilePath, containerFilePath)}
    case "dotnet":
        cmd = []string{"dotnet", containerFilePath}
    case "npm":
        cmd = []string{"npm", "run", containerFilePath}
    case "ng":
        cmd = []string{"ng", "run", containerFilePath}
    default:
        cmd = []string{"sh", "-c", fmt.Sprintf("chmod +x %s && %s", containerFilePath, containerFilePath)}
    }

    // Define container configuration
    containerConfig := &container.Config{
        Image: e.Image,
        Cmd:   cmd,
        Env:   envVars,
        Tty:   false,
    }

    hostConfig := &container.HostConfig{
        Mounts: []mount.Mount{
            {
                Type:   mount.TypeBind,
                Source: e.WorkDir,
                Target: "/workspace",
            },
        },
    }

    // Create the container
    resp, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to create container: %w", err),
        }
    }

    // Start the container
    if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to start container: %w", err),
        }
    }

    // Wait for the container to finish execution
    statusCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
    select {
    case err := <-errCh:
        if err != nil {
            return CodeResult{
                Output:   "",
                ExitCode: 1,
                Error:    fmt.Errorf("error while waiting for container: %w", err),
            }
        }
    case <-statusCh:
    }

    // Retrieve logs from the container
    out, err := cli.ContainerLogs(ctx, resp.ID, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true})
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to retrieve container logs: %w", err),
        }
    }
    defer out.Close()

    var stdoutBuf, stderrBuf bytes.Buffer
    _, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, out)
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to copy container output: %w", err),
        }
    }

    // Remove the container
    err = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
    if err != nil {
        return CodeResult{
            Output:   "",
            ExitCode: 1,
            Error:    fmt.Errorf("failed to remove container: %w", err),
        }
    }

    return CodeResult{
        Output:   stdoutBuf.String() + stderrBuf.String(),
        ExitCode: 0,
        Error:    nil,
    }
}