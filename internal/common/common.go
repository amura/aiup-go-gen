package common
type ToolCall struct {
    Name   string `json:"tool"`
    Args   map[string]interface{} `json:"args"` // Arguments for the tool, e.g. {"query": "search term"}
    Caller string
    Trace  []string // For tracking call stack or chains
    ID    string `json:"id,omitempty"` // Unique ID for the tool call
}

type ToolResult struct {
    Output interface{}
    Error error
    ErrorDetail  *ExecErrorDetail
	Name      string      `json:"name"`
	ErrorMsg  string      `json:"error_msg,omitempty"`
	ErrorType string      `json:"error_type,omitempty"`
	ErrorPhase string      `json:"error_phase,omitempty"`
	Role      string        `json:"role,omitempty"`
	Phase     string      `json:"phase,omitempty"`
	Command   string      `json:"command,omitempty"`
	CmdOutput string      `json:"cmd_output,omitempty"`
}

type ExecErrorDetail struct {
    Phase   string
    Command string
    Output  string
    ErrMsg  string
}