// internal/tools/generic_mcp_tool.go
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"aiupstart.com/go-gen/internal/common"
	"aiupstart.com/go-gen/internal/model"
)

type GenericMcpTool struct {
    NameStr        string // e.g. "stripe_mcp", "aws_mcp"
    Endpoint       string // e.g. "http://localhost:8080"
    DescriptionStr string // e.g. "Call Stripe MCP API..."
}

func (t *GenericMcpTool) Name() string        { return t.NameStr }
func (t *GenericMcpTool) Description() string { return t.DescriptionStr }

func (t *GenericMcpTool) Parameters() map[string]interface{} {
    return map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "path": map[string]interface{}{
                "type":        "string",
                "description": "API endpoint (e.g. /v1/customers)",
            },
            "method": map[string]interface{}{
                "type":        "string",
                "description": "HTTP method (GET, POST, ...)",
            },
            "body": map[string]interface{}{
                "type":        "string",
                "description": "JSON body for POST/PUT requests (object or string, optional)",
            },
        },
        "required": []string{"path", "method"},
    }
}

// This is the strict interface implementation required by Tool interface
func (t *GenericMcpTool) Call(ctx context.Context, call common.ToolCall) (common.ToolResult) {
    path, _ := call.Args["path"].(string)
    method, _ := call.Args["method"].(string)
    body := call.Args["body"]

    var bodyBytes []byte
    if body != nil {
        switch b := body.(type) {
        case string:
            bodyBytes = []byte(b)
        case map[string]interface{}:
            bodyBytes, _ = json.Marshal(b)
        }
    }

    url := t.Endpoint + path
    req, err := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
    if err != nil {
        return common.ToolResult{Role: model.RoleTool, Error: err}
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return common.ToolResult{Role: model.RoleTool, Error: err}
    }
    defer resp.Body.Close()
    data, _ := ioutil.ReadAll(resp.Body)
    if resp.StatusCode < 200 || resp.StatusCode > 299 {
        errMsg := fmt.Sprintf("MCP call failed: %s", resp.Status)
        return common.ToolResult{Role: model.RoleTool, Output: string(data), Error: fmt.Errorf("%s", errMsg)}
    }
    return common.ToolResult{Role: model.RoleTool, Output: string(data)}
}

func (t *GenericMcpTool) Run(ctx context.Context, call *common.ToolCall) (*model.ToolResult, error) {
    path, _ := call.Args["path"].(string)
    method, _ := call.Args["method"].(string)
    body := call.Args["body"]

    var bodyBytes []byte
    if body != nil {
        switch b := body.(type) {
        case string:
            bodyBytes = []byte(b)
        case map[string]interface{}:
            bodyBytes, _ = json.Marshal(b)
        }
    }

    url := t.Endpoint + path
    req, err := http.NewRequest(method, url, bytes.NewReader(bodyBytes))
    if err != nil {
        return &model.ToolResult{Role: model.RoleTool, Error: err.Error()},nil
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return &model.ToolResult{Role: model.RoleTool, Error: err.Error()}, nil
    }
    defer resp.Body.Close()
    data, _ := ioutil.ReadAll(resp.Body)
    if resp.StatusCode < 200 || resp.StatusCode > 299 {
        errMsg := fmt.Sprintf("MCP call failed: %s", resp.Status)
        return &model.ToolResult{Role: model.RoleTool, Output: string(data), Error: errMsg},nil
    }
    return &model.ToolResult{Role: model.RoleTool, Output: string(data)},nil
}