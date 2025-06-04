package config


import (
    "os"
    "gopkg.in/yaml.v3"
)

type McpToolOperation struct {
    Name        string                 `yaml:"name"`
    Path        string                 `yaml:"path"`
    Method      string                 `yaml:"method"`
    Description string                 `yaml:"description"`
    AllowedArgs  []string               `yaml:"allowed_args"`
    RequiredArgs []string               `yaml:"required_args"`
    ExampleArgs map[string]interface{} `yaml:"example_args"`
}
type McpToolConfig struct {
    Name        string              `yaml:"name"`
    Endpoint    string              `yaml:"endpoint"`
    Description string              `yaml:"description"`
    Operations  []McpToolOperation  `yaml:"operations"`
}
type McpConfig struct {
    McpTools []McpToolConfig `yaml:"mcp_tools"`
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