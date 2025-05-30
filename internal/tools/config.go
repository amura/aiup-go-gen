// internal/tools/config.go
package tools

import (
    "os"
    "gopkg.in/yaml.v2"
)

type ToolConfigEntry struct {
    Name    string `yaml:"name"`
    Enabled bool   `yaml:"enabled"`
}
type ToolConfig struct {
    Tools []ToolConfigEntry `yaml:"tools"`
}

func LoadToolConfig(path string) (*ToolConfig, error) {
    var cfg ToolConfig
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()
    dec := yaml.NewDecoder(f)
    err = dec.Decode(&cfg)
    return &cfg, err
}