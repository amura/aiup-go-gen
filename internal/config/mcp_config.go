package config

import (
	"fmt"
	"os"

	"aiupstart.com/go-gen/internal/utils"
	"gopkg.in/yaml.v3"
)

// type McpToolOperation struct {
//     Name        string                 `yaml:"name"`
//     Path        string                 `yaml:"path"`
//     Method      string                 `yaml:"method"`
//     Description string                 `yaml:"description"`
//     AllowedArgs  []string               `yaml:"allowed_args"`
//     RequiredArgs []string               `yaml:"required_args"`
//     ExampleArgs map[string]interface{} `yaml:"example_args"`
// }
type McpToolOperation struct {
    Name        string                 `yaml:"name" json:"name"`
    Path        string                 `yaml:"path" json:"path"`
    Method      string                 `yaml:"method" json:"method"`
    Description string                 `yaml:"description" json:"description"`
    ExampleArgs map[string]interface{} `yaml:"example_args" json:"example_args"`
    Parameters  map[string]interface{} `yaml:"parameters" json:"parameters"`
}
type McpToolConfig struct {
    Name        string              `yaml:"name" json:"name"`
    Endpoint    string              `yaml:"endpoint" json:"endpoint"`
    Description string              `yaml:"description" json:"description"`
    Operations  []McpToolOperation  `yaml:"operations" json:"operations"`
}
type McpConfig struct {
    McpTools []McpToolConfig `yaml:"mcp_tools" json:"mcp_tools"`
}


func LoadConfig(path string) (*McpConfig, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    var cfg McpConfig
    decoder := yaml.NewDecoder(f)
    if err := decoder.Decode(&cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
func ValidateMcpConfig(cfg *McpConfig) error {
    hasErr := false
    for _, tool := range cfg.McpTools {
        for _, op := range tool.Operations {
            params := op.Parameters // already map[string]interface{}
            if params == nil {
                utils.Logger.Error().
                    Str("tool", tool.Name).
                    Str("operation", op.Name).
                    Msg("Missing parameters block")
                hasErr = true
                continue
            }
            // Check type
            typ, _ := params["type"].(string)
            if typ != "object" {
                utils.Logger.Error().
                    Str("tool", tool.Name).
                    Str("operation", op.Name).
                    Msg("parameters.type must be 'object'")
                hasErr = true
            }
            // Check properties
            props, _ := params["properties"].(map[string]interface{})
            if props == nil || len(props) == 0 {
                utils.Logger.Error().
                    Str("tool", tool.Name).
                    Str("operation", op.Name).
                    Msg("parameters.properties missing or empty")
                hasErr = true
            }
            // Check required
            req, _ := params["required"].([]interface{})
            if req == nil || len(req) == 0 {
                utils.Logger.Error().
                    Str("tool", tool.Name).
                    Str("operation", op.Name).
                    Msg("parameters.required missing or empty")
                hasErr = true
            } else {
                for _, r := range req {
                    rStr, _ := r.(string)
                    if rStr == "" || props[rStr] == nil {
                        utils.Logger.Error().
                            Str("tool", tool.Name).
                            Str("operation", op.Name).
                            Str("required", rStr).
                            Msgf("Required property '%s' not in properties", rStr)
                        hasErr = true
                    }
                }
            }
        }
    }
    if hasErr {
        return fmt.Errorf("invalid MCP config: see above errors")
    }
    return nil
}