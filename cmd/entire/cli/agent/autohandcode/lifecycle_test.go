package autohandcode

import (
	"context"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

func TestParseHookEvent_SessionStart(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	input := `{"session_id": "test-session", "cwd": "/workspace", "hook_event_name": "session-start"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNameSessionStart, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event == nil {
		t.Fatal("expected event, got nil")
	}
	if event.Type != agent.SessionStart {
		t.Errorf("expected SessionStart, got %v", event.Type)
	}
	if event.SessionID != "test-session" {
		t.Errorf("expected session_id 'test-session', got %q", event.SessionID)
	}
	// SessionRef should be resolved from session_id
	if event.SessionRef == "" {
		t.Error("expected non-empty SessionRef")
	}
}

func TestParseHookEvent_TurnStart(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	input := `{"session_id": "sess-1", "cwd": "/workspace", "hook_event_name": "pre-prompt", "instruction": "Fix the bug"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNamePrePrompt, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != agent.TurnStart {
		t.Errorf("expected TurnStart, got %v", event.Type)
	}
	if event.Prompt != "Fix the bug" {
		t.Errorf("expected prompt 'Fix the bug', got %q", event.Prompt)
	}
	if event.SessionID != "sess-1" {
		t.Errorf("expected session_id 'sess-1', got %q", event.SessionID)
	}
}

func TestParseHookEvent_TurnStart_NoInstruction(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	// pre-prompt with no instruction field
	input := `{"session_id": "sess-empty", "cwd": "/workspace", "hook_event_name": "pre-prompt"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNamePrePrompt, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != agent.TurnStart {
		t.Errorf("expected TurnStart, got %v", event.Type)
	}
	if event.Prompt != "" {
		t.Errorf("expected empty prompt, got %q", event.Prompt)
	}
}

func TestParseHookEvent_TurnEnd(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	input := `{"session_id": "sess-2", "cwd": "/workspace", "hook_event_name": "stop", "tokens_used": 1000, "tool_calls_count": 5}`

	event, err := ag.ParseHookEvent(context.Background(), HookNameStop, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != agent.TurnEnd {
		t.Errorf("expected TurnEnd, got %v", event.Type)
	}
	if event.SessionID != "sess-2" {
		t.Errorf("expected session_id 'sess-2', got %q", event.SessionID)
	}
}

func TestParseHookEvent_SessionEnd(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	input := `{"session_id": "sess-3", "cwd": "/workspace", "hook_event_name": "session-end"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNameSessionEnd, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != agent.SessionEnd {
		t.Errorf("expected SessionEnd, got %v", event.Type)
	}
}

func TestParseHookEvent_SubagentEnd(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	input := `{"session_id": "sess-5", "cwd": "/workspace", "hook_event_name": "subagent-stop", "subagent_id": "agent-789", "subagent_name": "helper"}`

	event, err := ag.ParseHookEvent(context.Background(), HookNameSubagentStop, strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != agent.SubagentEnd {
		t.Errorf("expected SubagentEnd, got %v", event.Type)
	}
	if event.SubagentID != "agent-789" {
		t.Errorf("expected SubagentID 'agent-789', got %q", event.SubagentID)
	}
}

func TestParseHookEvent_Notification(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	event, err := ag.ParseHookEvent(context.Background(), HookNameNotification, strings.NewReader(`{"session_id":"s"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for notification, got %+v", event)
	}
}

func TestParseHookEvent_UnknownHook(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	event, err := ag.ParseHookEvent(context.Background(), "unknown-hook", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event != nil {
		t.Errorf("expected nil event for unknown hook, got %+v", event)
	}
}

func TestParseHookEvent_EmptyInput(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	_, err := ag.ParseHookEvent(context.Background(), HookNameSessionStart, strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseHookEvent_MalformedJSON(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	_, err := ag.ParseHookEvent(context.Background(), HookNameSessionStart, strings.NewReader("not json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestHookNames(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	names := ag.HookNames()

	expected := []string{
		HookNameSessionStart,
		HookNameSessionEnd,
		HookNameStop,
		HookNamePrePrompt,
		HookNameSubagentStop,
		HookNameNotification,
	}

	if len(names) != len(expected) {
		t.Fatalf("HookNames() returned %d names, want %d", len(names), len(expected))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, exp := range expected {
		if !nameSet[exp] {
			t.Errorf("HookNames() missing %q", exp)
		}
	}
}

func TestWriteHookResponse(t *testing.T) {
	t.Parallel()

	ag := &AutohandCodeAgent{}
	// WriteHookResponse writes to os.Stdout, so we just verify it does not error.
	// A more thorough test would capture stdout, but this is sufficient for coverage.
	err := ag.WriteHookResponse("test message")
	if err != nil {
		t.Fatalf("WriteHookResponse() error = %v", err)
	}
}

func TestParseHookEvent_AllHookTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		hookName     string
		input        string
		expectedType agent.EventType
		nilEvent     bool
	}{
		{
			name:         "session-start",
			hookName:     HookNameSessionStart,
			input:        `{"session_id":"s1","cwd":"/workspace","hook_event_name":"session-start"}`,
			expectedType: agent.SessionStart,
		},
		{
			name:         "pre-prompt",
			hookName:     HookNamePrePrompt,
			input:        `{"session_id":"s2","cwd":"/workspace","hook_event_name":"pre-prompt","instruction":"hello"}`,
			expectedType: agent.TurnStart,
		},
		{
			name:         "stop",
			hookName:     HookNameStop,
			input:        `{"session_id":"s3","cwd":"/workspace","hook_event_name":"stop","tokens_used":100}`,
			expectedType: agent.TurnEnd,
		},
		{
			name:         "session-end",
			hookName:     HookNameSessionEnd,
			input:        `{"session_id":"s4","cwd":"/workspace","hook_event_name":"session-end"}`,
			expectedType: agent.SessionEnd,
		},
		{
			name:         "subagent-stop",
			hookName:     HookNameSubagentStop,
			input:        `{"session_id":"s5","cwd":"/workspace","hook_event_name":"subagent-stop","subagent_id":"sub1"}`,
			expectedType: agent.SubagentEnd,
		},
		{
			name:     "notification returns nil",
			hookName: HookNameNotification,
			input:    `{"session_id":"s6","cwd":"/workspace","hook_event_name":"notification"}`,
			nilEvent: true,
		},
		{
			name:     "unknown returns nil",
			hookName: "totally-unknown",
			input:    `{"session_id":"s7"}`,
			nilEvent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ag := &AutohandCodeAgent{}
			event, err := ag.ParseHookEvent(context.Background(), tt.hookName, strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.nilEvent {
				if event != nil {
					t.Errorf("expected nil event, got %+v", event)
				}
				return
			}
			if event == nil {
				t.Fatal("expected event, got nil")
			}
			if event.Type != tt.expectedType {
				t.Errorf("expected %v, got %v", tt.expectedType, event.Type)
			}
		})
	}
}
