package autohandcode

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

func TestParseAutohandTranscript_BasicMessages(t *testing.T) {
	t.Parallel()

	data := []byte(
		`{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"hi there","timestamp":"2026-01-01T00:00:01.000Z"}` + "\n",
	)

	lines, totalLines, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("ParseAutohandTranscriptFromBytes() error = %v", err)
	}

	if totalLines != 2 {
		t.Errorf("totalLines = %d, want 2", totalLines)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}

	if lines[0].Type != transcript.TypeUser {
		t.Errorf("lines[0].Type = %q, want %q", lines[0].Type, transcript.TypeUser)
	}
	if lines[1].Type != transcript.TypeAssistant {
		t.Errorf("lines[1].Type = %q, want %q", lines[1].Type, transcript.TypeAssistant)
	}
}

func TestParseAutohandTranscript_FiltersNonUserAssistant(t *testing.T) {
	t.Parallel()

	// Autohand format has tool and system roles that should be filtered out
	data := []byte(
		`{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"I will create the file","timestamp":"2026-01-01T00:00:01.000Z","toolCalls":[{"id":"tc_1","name":"write_file","input":{"path":"foo.ts"}}]}` + "\n" +
			`{"role":"tool","content":"File written","name":"write_file","tool_call_id":"tc_1"}` + "\n" +
			`{"role":"system","content":"System prompt"}` + "\n" +
			`{"role":"assistant","content":"Done!","timestamp":"2026-01-01T00:00:02.000Z"}` + "\n",
	)

	lines, totalLines, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("ParseAutohandTranscriptFromBytes() error = %v", err)
	}

	if totalLines != 5 {
		t.Errorf("totalLines = %d, want 5", totalLines)
	}
	// Only user and assistant messages (tool and system should be filtered)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (tool and system should be filtered)", len(lines))
	}

	if lines[0].Type != transcript.TypeUser {
		t.Errorf("lines[0].Type = %q, want %q", lines[0].Type, transcript.TypeUser)
	}
	if lines[1].Type != transcript.TypeAssistant {
		t.Errorf("lines[1].Type = %q, want %q", lines[1].Type, transcript.TypeAssistant)
	}
	if lines[2].Type != transcript.TypeAssistant {
		t.Errorf("lines[2].Type = %q, want %q", lines[2].Type, transcript.TypeAssistant)
	}
}

func TestParseAutohandTranscript_StartLineOffset(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := tmpDir + "/transcript.jsonl"

	data := []byte(
		`{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"hi","timestamp":"2026-01-01T00:00:01.000Z"}` + "\n" +
			`{"role":"user","content":"bye","timestamp":"2026-01-01T00:00:02.000Z"}` + "\n",
	)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Read from line 1 onward (skip first message)
	lines, totalLines, err := ParseAutohandTranscript(path, 1)
	if err != nil {
		t.Fatalf("ParseAutohandTranscript() error = %v", err)
	}

	if totalLines != 3 {
		t.Errorf("totalLines = %d, want 3", totalLines)
	}

	if len(lines) != 2 {
		t.Fatalf("got %d lines from offset 1, want 2", len(lines))
	}
	if lines[0].Type != transcript.TypeAssistant {
		t.Errorf("lines[0].Type = %q, want %q", lines[0].Type, transcript.TypeAssistant)
	}
	if lines[1].Type != transcript.TypeUser {
		t.Errorf("lines[1].Type = %q, want %q", lines[1].Type, transcript.TypeUser)
	}
}

func TestParseAutohandTranscriptFromBytes_StartLineBeyondEnd(t *testing.T) {
	t.Parallel()

	data := []byte(
		`{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n",
	)

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 100)
	if err != nil {
		t.Fatalf("ParseAutohandTranscriptFromBytes(100) error = %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("startLine=100: got %d lines, want 0", len(lines))
	}
}

func TestParseAutohandTranscript_MalformedLines(t *testing.T) {
	t.Parallel()

	// Transcript with some broken JSON lines interspersed with valid ones
	data := []byte(
		`{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"broken json` + "\n" +
			`not even close to json` + "\n" +
			`{"role":"assistant","content":"hi","timestamp":"2026-01-01T00:00:01.000Z"}` + "\n",
	)

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("ParseAutohandTranscriptFromBytes() error = %v", err)
	}

	// Only the 2 valid user/assistant lines should be parsed
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (malformed lines should be silently skipped)", len(lines))
	}
	if lines[0].Type != transcript.TypeUser {
		t.Errorf("lines[0].Type = %q, want %q", lines[0].Type, transcript.TypeUser)
	}
	if lines[1].Type != transcript.TypeAssistant {
		t.Errorf("lines[1].Type = %q, want %q", lines[1].Type, transcript.TypeAssistant)
	}
}

func TestParseAutohandLine_Roles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantType string
		wantOK   bool
	}{
		{
			name:     "user role",
			input:    `{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}`,
			wantType: transcript.TypeUser,
			wantOK:   true,
		},
		{
			name:     "assistant role",
			input:    `{"role":"assistant","content":"hi","timestamp":"2026-01-01T00:00:01.000Z"}`,
			wantType: transcript.TypeAssistant,
			wantOK:   true,
		},
		{
			name:   "tool role filtered",
			input:  `{"role":"tool","content":"result","name":"write_file","tool_call_id":"tc_1"}`,
			wantOK: false,
		},
		{
			name:   "system role filtered",
			input:  `{"role":"system","content":"system prompt"}`,
			wantOK: false,
		},
		{
			name:   "empty JSON",
			input:  `{}`,
			wantOK: false,
		},
		{
			name:   "invalid JSON",
			input:  `not json`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			line, ok := parseAutohandLine([]byte(tt.input))
			if ok != tt.wantOK {
				t.Errorf("parseAutohandLine() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && line.Type != tt.wantType {
				t.Errorf("parseAutohandLine() type = %q, want %q", line.Type, tt.wantType)
			}
		})
	}
}

func TestParseAutohandLine_TimestampAsUUID(t *testing.T) {
	t.Parallel()

	input := []byte(`{"role":"user","content":"hello","timestamp":"2026-01-01T12:00:00.000Z"}`)
	line, ok := parseAutohandLine(input)
	if !ok {
		t.Fatal("expected ok = true")
	}
	// Autohand uses timestamp as pseudo-UUID since there are no message IDs
	if line.UUID != "2026-01-01T12:00:00.000Z" {
		t.Errorf("UUID = %q, want %q (timestamp as UUID)", line.UUID, "2026-01-01T12:00:00.000Z")
	}
}

// --- ExtractModifiedFiles tests ---

func TestExtractModifiedFiles_WriteFile(t *testing.T) {
	t.Parallel()

	data := []byte(`{"role":"assistant","content":"creating file","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"write_file","input":{"path":"foo.ts","content":"..."}}]}` + "\n")

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 1 || files[0] != "foo.ts" {
		t.Errorf("ExtractModifiedFiles() = %v, want [foo.ts]", files)
	}
}

func TestExtractModifiedFiles_EditFile(t *testing.T) {
	t.Parallel()

	data := []byte(`{"role":"assistant","content":"editing file","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"edit_file","input":{"path":"bar.go","old":"x","new":"y"}}]}` + "\n")

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 1 || files[0] != "bar.go" {
		t.Errorf("ExtractModifiedFiles() = %v, want [bar.go]", files)
	}
}

func TestExtractModifiedFiles_CreateFile(t *testing.T) {
	t.Parallel()

	data := []byte(`{"role":"assistant","content":"creating file","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"create_file","input":{"path":"new.py","content":"..."}}]}` + "\n")

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 1 || files[0] != "new.py" {
		t.Errorf("ExtractModifiedFiles() = %v, want [new.py]", files)
	}
}

func TestExtractModifiedFiles_PatchFile(t *testing.T) {
	t.Parallel()

	data := []byte(`{"role":"assistant","content":"patching file","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"patch_file","input":{"path":"app.js","patch":"..."}}]}` + "\n")

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 1 || files[0] != "app.js" {
		t.Errorf("ExtractModifiedFiles() = %v, want [app.js]", files)
	}
}

func TestExtractModifiedFiles_FileFieldFallback(t *testing.T) {
	t.Parallel()

	// Some tools use "file" instead of "path"
	data := []byte(`{"role":"assistant","content":"creating","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"write_file","input":{"file":"alt.txt","content":"..."}}]}` + "\n")

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 1 || files[0] != "alt.txt" {
		t.Errorf("ExtractModifiedFiles() = %v, want [alt.txt]", files)
	}
}

func TestExtractModifiedFiles_MultipleToolCalls(t *testing.T) {
	t.Parallel()

	data := []byte(
		`{"role":"assistant","content":"creating files","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"write_file","input":{"path":"foo.go"}},{"id":"tc_2","name":"edit_file","input":{"path":"bar.go"}}]}` + "\n" +
			`{"role":"assistant","content":"more","timestamp":"2026-01-01T00:00:01.000Z","toolCalls":[{"id":"tc_3","name":"create_file","input":{"path":"baz.go"}}]}` + "\n",
	)

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 3 {
		t.Fatalf("ExtractModifiedFiles() got %d files, want 3", len(files))
	}

	hasFile := func(name string) bool {
		for _, f := range files {
			if f == name {
				return true
			}
		}
		return false
	}

	if !hasFile("foo.go") {
		t.Error("missing foo.go")
	}
	if !hasFile("bar.go") {
		t.Error("missing bar.go")
	}
	if !hasFile("baz.go") {
		t.Error("missing baz.go")
	}
}

func TestExtractModifiedFiles_Deduplication(t *testing.T) {
	t.Parallel()

	data := []byte(
		`{"role":"assistant","content":"first write","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"write_file","input":{"path":"same.go"}}]}` + "\n" +
			`{"role":"assistant","content":"second write","timestamp":"2026-01-01T00:00:01.000Z","toolCalls":[{"id":"tc_2","name":"write_file","input":{"path":"same.go"}}]}` + "\n",
	)

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 1 {
		t.Errorf("ExtractModifiedFiles() got %d files, want 1 (deduplicated)", len(files))
	}
}

func TestExtractModifiedFiles_IgnoresNonFileTools(t *testing.T) {
	t.Parallel()

	data := []byte(
		`{"role":"assistant","content":"running command","timestamp":"2026-01-01T00:00:00.000Z","toolCalls":[{"id":"tc_1","name":"exec_command","input":{"command":"ls"}}]}` + "\n" +
			`{"role":"assistant","content":"reading","timestamp":"2026-01-01T00:00:01.000Z","toolCalls":[{"id":"tc_2","name":"read_file","input":{"path":"readme.md"}}]}` + "\n",
	)

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	files := ExtractModifiedFiles(lines)

	if len(files) != 0 {
		t.Errorf("ExtractModifiedFiles() = %v, want empty (non-file tools should be ignored)", files)
	}
}

func TestExtractModifiedFiles_Empty(t *testing.T) {
	t.Parallel()

	files := ExtractModifiedFiles(nil)
	if files != nil {
		t.Errorf("ExtractModifiedFiles(nil) = %v, want nil", files)
	}
}

// --- CalculateTokenUsage tests ---

func TestCalculateTokenUsage_WithMeta(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type: transcript.TypeAssistant,
			UUID: "ts1",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"first response"`),
				Meta: map[string]any{
					"input_tokens":                float64(100),
					"output_tokens":               float64(50),
					"cache_creation_input_tokens": float64(200),
					"cache_read_input_tokens":     float64(30),
				},
			}),
		},
		{
			Type: transcript.TypeAssistant,
			UUID: "ts2",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"second response"`),
				Meta: map[string]any{
					"input_tokens":  float64(50),
					"output_tokens": float64(25),
				},
			}),
		},
	}

	usage := CalculateTokenUsage(lines)

	if usage.APICallCount != 2 {
		t.Errorf("APICallCount = %d, want 2", usage.APICallCount)
	}
	if usage.InputTokens != 150 {
		t.Errorf("InputTokens = %d, want 150", usage.InputTokens)
	}
	if usage.OutputTokens != 75 {
		t.Errorf("OutputTokens = %d, want 75", usage.OutputTokens)
	}
	if usage.CacheCreationTokens != 200 {
		t.Errorf("CacheCreationTokens = %d, want 200", usage.CacheCreationTokens)
	}
	if usage.CacheReadTokens != 30 {
		t.Errorf("CacheReadTokens = %d, want 30", usage.CacheReadTokens)
	}
}

func TestCalculateTokenUsage_NoMeta(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type: transcript.TypeAssistant,
			UUID: "ts1",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"response without meta"`),
			}),
		},
	}

	usage := CalculateTokenUsage(lines)

	if usage.APICallCount != 0 {
		t.Errorf("APICallCount = %d, want 0 (no _meta means no API call counted)", usage.APICallCount)
	}
	if usage.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0", usage.InputTokens)
	}
}

func TestCalculateTokenUsage_IgnoresUserMessages(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type:    transcript.TypeUser,
			UUID:    "ts1",
			Message: mustMarshal(t, autohandMessage{Role: "user", Content: json.RawMessage(`"hello"`)}),
		},
		{
			Type: transcript.TypeAssistant,
			UUID: "ts2",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"response"`),
				Meta: map[string]any{
					"input_tokens":  float64(10),
					"output_tokens": float64(20),
				},
			}),
		},
	}

	usage := CalculateTokenUsage(lines)

	if usage.APICallCount != 1 {
		t.Errorf("APICallCount = %d, want 1", usage.APICallCount)
	}
}

func TestCalculateTokenUsageFromFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	content :=
		`{"role":"user","content":"hello","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"response","timestamp":"2026-01-01T00:00:01.000Z","_meta":{"input_tokens":100,"output_tokens":50}}` + "\n" +
			`{"role":"user","content":"bye","timestamp":"2026-01-01T00:00:02.000Z"}` + "\n" +
			`{"role":"assistant","content":"goodbye","timestamp":"2026-01-01T00:00:03.000Z","_meta":{"input_tokens":200,"output_tokens":100}}` + "\n"

	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	// Test from line 0 - all turns
	usage1, err := CalculateTokenUsageFromFile(transcriptPath, 0)
	if err != nil {
		t.Fatalf("CalculateTokenUsageFromFile(0) error: %v", err)
	}
	if usage1.InputTokens != 300 {
		t.Errorf("From line 0: InputTokens = %d, want 300", usage1.InputTokens)
	}
	if usage1.OutputTokens != 150 {
		t.Errorf("From line 0: OutputTokens = %d, want 150", usage1.OutputTokens)
	}
	if usage1.APICallCount != 2 {
		t.Errorf("From line 0: APICallCount = %d, want 2", usage1.APICallCount)
	}

	// Test from line 2 - second turn only
	usage2, err := CalculateTokenUsageFromFile(transcriptPath, 2)
	if err != nil {
		t.Fatalf("CalculateTokenUsageFromFile(2) error: %v", err)
	}
	if usage2.InputTokens != 200 {
		t.Errorf("From line 2: InputTokens = %d, want 200", usage2.InputTokens)
	}
	if usage2.OutputTokens != 100 {
		t.Errorf("From line 2: OutputTokens = %d, want 100", usage2.OutputTokens)
	}
	if usage2.APICallCount != 1 {
		t.Errorf("From line 2: APICallCount = %d, want 1", usage2.APICallCount)
	}
}

func TestCalculateTokenUsageFromFile_EmptyPath(t *testing.T) {
	t.Parallel()

	usage, err := CalculateTokenUsageFromFile("", 0)
	if err != nil {
		t.Fatalf("CalculateTokenUsageFromFile('') error: %v", err)
	}
	if usage.APICallCount != 0 {
		t.Errorf("APICallCount = %d, want 0", usage.APICallCount)
	}
}

// --- ExtractSpawnedAgentIDs tests ---

func TestExtractSpawnedAgentIDs_FromTaskCalls(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type: transcript.TypeAssistant,
			UUID: "ts1",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"spawning agent"`),
				ToolCalls: []autohandToolCall{
					{ID: "agent-abc", Name: "Task", Input: json.RawMessage(`{"prompt":"do something"}`)},
				},
			}),
		},
	}

	agentIDs := ExtractSpawnedAgentIDs(lines)

	if len(agentIDs) != 1 {
		t.Fatalf("Expected 1 agent ID, got %d", len(agentIDs))
	}
	if _, ok := agentIDs["agent-abc"]; !ok {
		t.Errorf("Expected agent ID 'agent-abc', got %v", agentIDs)
	}
}

func TestExtractSpawnedAgentIDs_LowercaseTask(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type: transcript.TypeAssistant,
			UUID: "ts1",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"spawning"`),
				ToolCalls: []autohandToolCall{
					{ID: "agent-def", Name: "task", Input: json.RawMessage(`{"prompt":"hello"}`)},
				},
			}),
		},
	}

	agentIDs := ExtractSpawnedAgentIDs(lines)

	if len(agentIDs) != 1 {
		t.Fatalf("Expected 1 agent ID, got %d", len(agentIDs))
	}
	if _, ok := agentIDs["agent-def"]; !ok {
		t.Errorf("Expected agent ID 'agent-def', got %v", agentIDs)
	}
}

func TestExtractSpawnedAgentIDs_MultipleAgents(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type: transcript.TypeAssistant,
			UUID: "ts1",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"first task"`),
				ToolCalls: []autohandToolCall{
					{ID: "agent-1", Name: "Task", Input: json.RawMessage(`{}`)},
				},
			}),
		},
		{
			Type: transcript.TypeAssistant,
			UUID: "ts2",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"second task"`),
				ToolCalls: []autohandToolCall{
					{ID: "agent-2", Name: "Task", Input: json.RawMessage(`{}`)},
				},
			}),
		},
	}

	agentIDs := ExtractSpawnedAgentIDs(lines)

	if len(agentIDs) != 2 {
		t.Fatalf("Expected 2 agent IDs, got %d", len(agentIDs))
	}
}

func TestExtractSpawnedAgentIDs_NoTaskCalls(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type: transcript.TypeAssistant,
			UUID: "ts1",
			Message: mustMarshal(t, autohandMessage{
				Role:    "assistant",
				Content: json.RawMessage(`"just writing"`),
				ToolCalls: []autohandToolCall{
					{ID: "tc_1", Name: "write_file", Input: json.RawMessage(`{"path":"foo.go"}`)},
				},
			}),
		},
	}

	agentIDs := ExtractSpawnedAgentIDs(lines)

	if len(agentIDs) != 0 {
		t.Errorf("Expected 0 agent IDs, got %d: %v", len(agentIDs), agentIDs)
	}
}

func TestExtractSpawnedAgentIDs_IgnoresUserMessages(t *testing.T) {
	t.Parallel()

	lines := []TranscriptLine{
		{
			Type:    transcript.TypeUser,
			UUID:    "ts1",
			Message: mustMarshal(t, autohandMessage{Role: "user", Content: json.RawMessage(`"hello"`)}),
		},
	}

	agentIDs := ExtractSpawnedAgentIDs(lines)

	if len(agentIDs) != 0 {
		t.Errorf("Expected 0 agent IDs, got %d", len(agentIDs))
	}
}

// --- ExtractAllModifiedFilesFromBytes tests ---

func TestExtractAllModifiedFilesFromBytes_IncludesSubagentFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subagentsDir := tmpDir + "/tasks"

	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		t.Fatalf("failed to create subagents dir: %v", err)
	}

	// Main transcript: Write to main.go + Task call spawning subagent
	mainTranscript := makeAutohandWriteToolLine(t, "2026-01-01T00:00:00.000Z", "/repo/main.go") + "\n" +
		makeAutohandTaskToolLine(t, "2026-01-01T00:00:01.000Z", "sub1") + "\n"

	// Subagent transcript: Write to helper.go + Edit to utils.go
	subTranscript := makeAutohandWriteToolLine(t, "2026-01-01T00:00:02.000Z", "/repo/helper.go") + "\n" +
		makeAutohandEditToolLine(t, "2026-01-01T00:00:03.000Z", "/repo/utils.go") + "\n"
	if err := os.WriteFile(subagentsDir+"/agent-sub1.jsonl", []byte(subTranscript), 0o600); err != nil {
		t.Fatalf("failed to write subagent transcript: %v", err)
	}

	files, err := ExtractAllModifiedFilesFromBytes([]byte(mainTranscript), 0, subagentsDir)
	if err != nil {
		t.Fatalf("ExtractAllModifiedFilesFromBytes() error: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}

	wantFiles := map[string]bool{
		"/repo/main.go":   true,
		"/repo/helper.go": true,
		"/repo/utils.go":  true,
	}
	for _, f := range files {
		if !wantFiles[f] {
			t.Errorf("unexpected file %q in result", f)
		}
		delete(wantFiles, f)
	}
	for f := range wantFiles {
		t.Errorf("missing expected file %q", f)
	}
}

func TestExtractAllModifiedFilesFromBytes_DeduplicatesAcrossAgents(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subagentsDir := tmpDir + "/tasks"

	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		t.Fatalf("failed to create subagents dir: %v", err)
	}

	mainTranscript := makeAutohandWriteToolLine(t, "2026-01-01T00:00:00.000Z", "/repo/shared.go") + "\n" +
		makeAutohandTaskToolLine(t, "2026-01-01T00:00:01.000Z", "sub1") + "\n"

	subTranscript := makeAutohandEditToolLine(t, "2026-01-01T00:00:02.000Z", "/repo/shared.go") + "\n"
	if err := os.WriteFile(subagentsDir+"/agent-sub1.jsonl", []byte(subTranscript), 0o600); err != nil {
		t.Fatalf("failed to write subagent transcript: %v", err)
	}

	files, err := ExtractAllModifiedFilesFromBytes([]byte(mainTranscript), 0, subagentsDir)
	if err != nil {
		t.Fatalf("ExtractAllModifiedFilesFromBytes() error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file (deduplicated), got %d: %v", len(files), files)
	}
}

func TestExtractAllModifiedFilesFromBytes_NoSubagents(t *testing.T) {
	t.Parallel()

	mainTranscript := makeAutohandWriteToolLine(t, "2026-01-01T00:00:00.000Z", "/repo/solo.go") + "\n"

	files, err := ExtractAllModifiedFilesFromBytes([]byte(mainTranscript), 0, "/nonexistent")
	if err != nil {
		t.Fatalf("ExtractAllModifiedFilesFromBytes() error: %v", err)
	}

	if len(files) != 1 || files[0] != "/repo/solo.go" {
		t.Errorf("expected [/repo/solo.go], got %v", files)
	}
}

func TestExtractAllModifiedFilesFromBytes_EmptyData(t *testing.T) {
	t.Parallel()

	files, err := ExtractAllModifiedFilesFromBytes(nil, 0, "")
	if err != nil {
		t.Fatalf("ExtractAllModifiedFilesFromBytes() error: %v", err)
	}
	if files != nil {
		t.Errorf("expected nil, got %v", files)
	}
}

// --- CalculateTotalTokenUsageFromBytes tests ---

func TestCalculateTotalTokenUsageFromBytes_WithSubagents(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subagentsDir := tmpDir + "/tasks"

	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		t.Fatalf("failed to create subagents dir: %v", err)
	}

	mainTranscript := makeAutohandTokenLine(t, "2026-01-01T00:00:00.000Z", 100, 50) + "\n" +
		makeAutohandTaskToolLine(t, "2026-01-01T00:00:01.000Z", "sub1") + "\n"

	subTranscript := makeAutohandTokenLine(t, "2026-01-01T00:00:02.000Z", 200, 80) + "\n" +
		makeAutohandTokenLine(t, "2026-01-01T00:00:03.000Z", 150, 60) + "\n"
	if err := os.WriteFile(subagentsDir+"/agent-sub1.jsonl", []byte(subTranscript), 0o600); err != nil {
		t.Fatalf("failed to write subagent transcript: %v", err)
	}

	usage, err := CalculateTotalTokenUsageFromBytes([]byte(mainTranscript), 0, subagentsDir)
	if err != nil {
		t.Fatalf("CalculateTotalTokenUsageFromBytes() error: %v", err)
	}

	// Main agent tokens
	if usage.InputTokens != 100 {
		t.Errorf("main InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("main OutputTokens = %d, want 50", usage.OutputTokens)
	}
	if usage.APICallCount != 1 {
		t.Errorf("main APICallCount = %d, want 1", usage.APICallCount)
	}

	// Subagent tokens
	if usage.SubagentTokens == nil {
		t.Fatal("SubagentTokens is nil")
	}
	if usage.SubagentTokens.InputTokens != 350 {
		t.Errorf("subagent InputTokens = %d, want 350 (200+150)", usage.SubagentTokens.InputTokens)
	}
	if usage.SubagentTokens.OutputTokens != 140 {
		t.Errorf("subagent OutputTokens = %d, want 140 (80+60)", usage.SubagentTokens.OutputTokens)
	}
	if usage.SubagentTokens.APICallCount != 2 {
		t.Errorf("subagent APICallCount = %d, want 2", usage.SubagentTokens.APICallCount)
	}
}

func TestCalculateTotalTokenUsageFromBytes_EmptyData(t *testing.T) {
	t.Parallel()

	usage, err := CalculateTotalTokenUsageFromBytes(nil, 0, "")
	if err != nil {
		t.Fatalf("CalculateTotalTokenUsageFromBytes() error: %v", err)
	}
	if usage.APICallCount != 0 {
		t.Errorf("APICallCount = %d, want 0", usage.APICallCount)
	}
}

// --- ExtractPrompts tests ---

func TestExtractPrompts(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	content :=
		`{"role":"user","content":"Fix the login bug","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"I'll fix the bug.","timestamp":"2026-01-01T00:00:01.000Z"}` + "\n" +
			`{"role":"user","content":"Now add tests","timestamp":"2026-01-01T00:00:02.000Z"}` + "\n"

	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	ag := &AutohandCodeAgent{}
	prompts, err := ag.ExtractPrompts(transcriptPath, 0)
	if err != nil {
		t.Fatalf("ExtractPrompts() error = %v", err)
	}

	if len(prompts) != 2 {
		t.Fatalf("ExtractPrompts() got %d prompts, want 2", len(prompts))
	}
	if prompts[0] != "Fix the login bug" {
		t.Errorf("prompts[0] = %q, want %q", prompts[0], "Fix the login bug")
	}
	if prompts[1] != "Now add tests" {
		t.Errorf("prompts[1] = %q, want %q", prompts[1], "Now add tests")
	}
}

func TestExtractPrompts_WithOffset(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	content :=
		`{"role":"user","content":"First prompt","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"Done.","timestamp":"2026-01-01T00:00:01.000Z"}` + "\n" +
			`{"role":"user","content":"Second prompt","timestamp":"2026-01-01T00:00:02.000Z"}` + "\n" +
			`{"role":"assistant","content":"Done again.","timestamp":"2026-01-01T00:00:03.000Z"}` + "\n"

	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	ag := &AutohandCodeAgent{}
	// Skip first 2 lines (first user+assistant turn)
	prompts, err := ag.ExtractPrompts(transcriptPath, 2)
	if err != nil {
		t.Fatalf("ExtractPrompts() error = %v", err)
	}

	if len(prompts) != 1 {
		t.Fatalf("ExtractPrompts() got %d prompts, want 1", len(prompts))
	}
	if prompts[0] != "Second prompt" {
		t.Errorf("prompts[0] = %q, want %q", prompts[0], "Second prompt")
	}
}

// --- ExtractSummary tests ---

func TestExtractSummary(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"

	content :=
		`{"role":"user","content":"Fix the bug","timestamp":"2026-01-01T00:00:00.000Z"}` + "\n" +
			`{"role":"assistant","content":"Working on it...","timestamp":"2026-01-01T00:00:01.000Z"}` + "\n" +
			`{"role":"user","content":"Thanks","timestamp":"2026-01-01T00:00:02.000Z"}` + "\n" +
			`{"role":"assistant","content":"All done! The bug is fixed.","timestamp":"2026-01-01T00:00:03.000Z"}` + "\n"

	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write transcript: %v", err)
	}

	ag := &AutohandCodeAgent{}
	summary, err := ag.ExtractSummary(transcriptPath)
	if err != nil {
		t.Fatalf("ExtractSummary() error = %v", err)
	}

	if summary != "All done! The bug is fixed." {
		t.Errorf("ExtractSummary() = %q, want %q", summary, "All done! The bug is fixed.")
	}
}

func TestExtractSummary_EmptyTranscript(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	transcriptPath := tmpDir + "/transcript.jsonl"
	if err := os.WriteFile(transcriptPath, []byte(""), 0o600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	ag := &AutohandCodeAgent{}
	summary, err := ag.ExtractSummary(transcriptPath)
	if err != nil {
		t.Fatalf("ExtractSummary() error = %v", err)
	}

	if summary != "" {
		t.Errorf("ExtractSummary() = %q, want empty string", summary)
	}
}

// --- Test helpers ---

// mustMarshal is a test helper that marshals a value to JSON or fails the test.
func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return data
}

// makeAutohandWriteToolLine returns an Autohand-format JSONL line with a write_file tool call.
func makeAutohandWriteToolLine(t *testing.T, timestamp, filePath string) string {
	t.Helper()
	msg := autohandMessage{
		Role:      "assistant",
		Content:   json.RawMessage(`"creating file"`),
		Timestamp: timestamp,
		ToolCalls: []autohandToolCall{
			{
				ID:    "tc_" + timestamp,
				Name:  ToolWriteFile,
				Input: json.RawMessage(`{"path":"` + filePath + `","content":"..."}`),
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return string(data)
}

// makeAutohandEditToolLine returns an Autohand-format JSONL line with an edit_file tool call.
func makeAutohandEditToolLine(t *testing.T, timestamp, filePath string) string {
	t.Helper()
	msg := autohandMessage{
		Role:      "assistant",
		Content:   json.RawMessage(`"editing file"`),
		Timestamp: timestamp,
		ToolCalls: []autohandToolCall{
			{
				ID:    "tc_" + timestamp,
				Name:  ToolEditFile,
				Input: json.RawMessage(`{"path":"` + filePath + `","old":"x","new":"y"}`),
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return string(data)
}

// makeAutohandTaskToolLine returns an Autohand-format JSONL line with a Task tool call.
func makeAutohandTaskToolLine(t *testing.T, timestamp, agentID string) string {
	t.Helper()
	msg := autohandMessage{
		Role:      "assistant",
		Content:   json.RawMessage(`"spawning subagent"`),
		Timestamp: timestamp,
		ToolCalls: []autohandToolCall{
			{
				ID:    agentID,
				Name:  "Task",
				Input: json.RawMessage(`{"prompt":"do something"}`),
			},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return string(data)
}

// makeAutohandTokenLine returns an Autohand-format JSONL line with _meta token data.
func makeAutohandTokenLine(t *testing.T, timestamp string, inputTokens, outputTokens int) string {
	t.Helper()

	// Build the _meta map with proper typing for JSON marshal/unmarshal
	meta := map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	}

	msg := map[string]any{
		"role":      "assistant",
		"content":   "response",
		"timestamp": timestamp,
		"_meta":     meta,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return string(data)
}

// writeJSONLFile is a test helper that writes JSONL transcript lines to a file.
func writeJSONLFile(t *testing.T, path string, lines ...string) {
	t.Helper()
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(buf.String()), 0o600); err != nil {
		t.Fatalf("failed to write JSONL file %s: %v", path, err)
	}
}
