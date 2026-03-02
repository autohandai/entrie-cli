package autohandcode

import "encoding/json"

// AutohandConfig represents the .autohand/config.json structure.
type AutohandConfig struct {
	Hooks AutohandHooksSettings `json:"hooks"`
}

// AutohandHooksSettings contains the hooks configuration.
type AutohandHooksSettings struct {
	Enabled *bool             `json:"enabled,omitempty"`
	Hooks   []AutohandHookDef `json:"hooks,omitempty"`
}

// AutohandHookDef represents a single hook definition.
type AutohandHookDef struct {
	Event       string          `json:"event"`
	Command     string          `json:"command"`
	Description string          `json:"description,omitempty"`
	Enabled     *bool           `json:"enabled,omitempty"`
	Timeout     int             `json:"timeout,omitempty"`
	Async       bool            `json:"async,omitempty"`
	Matcher     string          `json:"matcher,omitempty"`
	Filter      *AutohandFilter `json:"filter,omitempty"`
}

// AutohandFilter limits when a hook fires.
type AutohandFilter struct {
	Tool []string `json:"tool,omitempty"`
	Path []string `json:"path,omitempty"`
}

// AutohandPermissionRule represents a permission rule in config.
type AutohandPermissionRule struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern,omitempty"`
	Action  string `json:"action"`
}

// hookInputBase is the JSON structure piped to hook stdin for all events.
type hookInputBase struct {
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
}

// hookInputPrompt extends hookInputBase for pre-prompt events.
type hookInputPrompt struct {
	SessionID      string   `json:"session_id"`
	CWD            string   `json:"cwd"`
	HookEventName  string   `json:"hook_event_name"`
	Instruction    string   `json:"instruction"`
	MentionedFiles []string `json:"mentioned_files"`
}

// hookInputStop extends hookInputBase for stop/post-response events.
type hookInputStop struct {
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	TokensUsed     int    `json:"tokens_used"`
	ToolCallsCount int    `json:"tool_calls_count"`
	TurnToolCalls  int    `json:"turn_tool_calls"`
	TurnDuration   int    `json:"turn_duration"`
	Duration       int    `json:"duration"`
}

// hookInputTool extends hookInputBase for pre-tool/post-tool events.
type hookInputTool struct {
	SessionID     string          `json:"session_id"`
	CWD           string          `json:"cwd"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolUseID     string          `json:"tool_use_id"`
	ToolResponse  string          `json:"tool_response"`
	ToolSuccess   *bool           `json:"tool_success"`
}

// hookInputSubagent extends hookInputBase for subagent-stop events.
type hookInputSubagent struct {
	SessionID        string `json:"session_id"`
	CWD              string `json:"cwd"`
	HookEventName    string `json:"hook_event_name"`
	SubagentID       string `json:"subagent_id"`
	SubagentName     string `json:"subagent_name"`
	SubagentType     string `json:"subagent_type"`
	SubagentSuccess  *bool  `json:"subagent_success"`
	SubagentError    string `json:"subagent_error"`
	SubagentDuration int    `json:"subagent_duration"`
}

// Tool names used in Autohand Code transcripts for file modification.
const (
	ToolWriteFile  = "write_file"
	ToolEditFile   = "edit_file"
	ToolCreateFile = "create_file"
	ToolPatchFile  = "patch_file"
)

// FileModificationTools lists tools that create or modify files.
var FileModificationTools = []string{
	ToolWriteFile,
	ToolEditFile,
	ToolCreateFile,
	ToolPatchFile,
}

// autohandToolCall represents a tool call in the Autohand transcript format.
type autohandToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// autohandToolInput represents the input to a file modification tool.
type autohandToolInput struct {
	Path string `json:"path"`
	File string `json:"file"`
}

// autohandMessage represents a single JSONL line in the Autohand transcript.
type autohandMessage struct {
	Role       string             `json:"role"`
	Content    json.RawMessage    `json:"content"`
	Timestamp  string             `json:"timestamp"`
	ToolCalls  []autohandToolCall `json:"toolCalls,omitempty"`
	Name       string             `json:"name,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	Meta       map[string]any     `json:"_meta,omitempty"`
}

// autohandTokenMeta is extracted from _meta for token tracking.
type autohandTokenMeta struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
