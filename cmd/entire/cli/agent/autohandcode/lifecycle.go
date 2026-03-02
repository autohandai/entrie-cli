package autohandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/textutil"
	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

// Compile-time interface assertions.
var (
	_ agent.TranscriptAnalyzer     = (*AutohandCodeAgent)(nil)
	_ agent.TokenCalculator        = (*AutohandCodeAgent)(nil)
	_ agent.SubagentAwareExtractor = (*AutohandCodeAgent)(nil)
	_ agent.HookResponseWriter     = (*AutohandCodeAgent)(nil)
)

// WriteHookResponse outputs the hook response as plain text to stdout.
// Autohand parses JSON stdout only for control flow (pre-tool hooks).
// For lifecycle hooks, plain text is safe and displays in the terminal.
func (a *AutohandCodeAgent) WriteHookResponse(message string) error {
	if _, err := fmt.Fprintln(os.Stdout, message); err != nil {
		return fmt.Errorf("failed to write hook response: %w", err)
	}
	return nil
}

// HookNames returns the hook verbs Autohand Code supports.
// These become subcommands: entire hooks autohand-code <verb>
func (a *AutohandCodeAgent) HookNames() []string {
	return []string{
		HookNameSessionStart,
		HookNameSessionEnd,
		HookNameStop,
		HookNamePrePrompt,
		HookNameSubagentStop,
		HookNameNotification,
	}
}

// ParseHookEvent translates an Autohand Code hook into a normalized lifecycle Event.
// Returns nil if the hook has no lifecycle significance.
func (a *AutohandCodeAgent) ParseHookEvent(_ context.Context, hookName string, stdin io.Reader) (*agent.Event, error) {
	switch hookName {
	case HookNameSessionStart:
		return a.parseSessionStart(stdin)
	case HookNamePrePrompt:
		return a.parseTurnStart(stdin)
	case HookNameStop:
		return a.parseTurnEnd(stdin)
	case HookNameSessionEnd:
		return a.parseSessionEnd(stdin)
	case HookNameSubagentStop:
		return a.parseSubagentEnd(stdin)
	case HookNameNotification:
		// Acknowledged hook with no lifecycle action
		return nil, nil //nolint:nilnil // nil event = no lifecycle action
	default:
		return nil, nil //nolint:nilnil // Unknown hooks have no lifecycle action
	}
}

// --- TranscriptAnalyzer ---

// GetTranscriptPosition returns the current line count of the JSONL transcript.
func (a *AutohandCodeAgent) GetTranscriptPosition(path string) (int, error) {
	_, pos, err := ParseAutohandTranscript(path, 0)
	if err != nil {
		return 0, err
	}
	return pos, nil
}

// ExtractModifiedFilesFromOffset extracts files modified since a given line offset.
func (a *AutohandCodeAgent) ExtractModifiedFilesFromOffset(path string, startOffset int) ([]string, int, error) {
	lines, currentPos, err := ParseAutohandTranscript(path, startOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse transcript: %w", err)
	}
	files := ExtractModifiedFiles(lines)
	return files, currentPos, nil
}

// ExtractPrompts extracts user prompts from the transcript starting at the given line offset.
func (a *AutohandCodeAgent) ExtractPrompts(sessionRef string, fromOffset int) ([]string, error) {
	lines, _, err := ParseAutohandTranscript(sessionRef, fromOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	var prompts []string
	for i := range lines {
		if lines[i].Type != transcript.TypeUser {
			continue
		}
		content := transcript.ExtractUserContent(lines[i].Message)
		if content != "" {
			prompts = append(prompts, textutil.StripIDEContextTags(content))
		}
	}
	return prompts, nil
}

// ExtractSummary extracts the last assistant message as a session summary.
func (a *AutohandCodeAgent) ExtractSummary(sessionRef string) (string, error) {
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path comes from agent hook input
	if err != nil {
		return "", fmt.Errorf("failed to read transcript: %w", err)
	}
	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		return "", fmt.Errorf("failed to parse transcript: %w", err)
	}

	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Type != transcript.TypeAssistant {
			continue
		}
		// For Autohand, the Message field contains the full message object.
		// Try to extract the content field as a plain text string.
		var msg autohandMessage
		if err := json.Unmarshal(lines[i].Message, &msg); err != nil {
			continue
		}
		var textContent string
		if err := json.Unmarshal(msg.Content, &textContent); err == nil && textContent != "" {
			return textContent, nil
		}
		// Also try the shared AssistantMessage format (content blocks)
		var assistantMsg transcript.AssistantMessage
		if err := json.Unmarshal(lines[i].Message, &assistantMsg); err != nil {
			continue
		}
		for _, block := range assistantMsg.Content {
			if block.Type == transcript.ContentTypeText && block.Text != "" {
				return block.Text, nil
			}
		}
	}
	return "", nil
}

// --- TokenCalculator ---

// CalculateTokenUsage computes token usage from pre-loaded transcript bytes starting at the given line offset.
func (a *AutohandCodeAgent) CalculateTokenUsage(transcriptData []byte, fromOffset int) (*agent.TokenUsage, error) {
	return CalculateTotalTokenUsageFromBytes(transcriptData, fromOffset, "")
}

// --- SubagentAwareExtractor ---

// ExtractAllModifiedFiles extracts files modified by both the main agent and any spawned subagents.
func (a *AutohandCodeAgent) ExtractAllModifiedFiles(transcriptData []byte, fromOffset int, subagentsDir string) ([]string, error) {
	return ExtractAllModifiedFilesFromBytes(transcriptData, fromOffset, subagentsDir)
}

// CalculateTotalTokenUsage computes token usage including all spawned subagents.
func (a *AutohandCodeAgent) CalculateTotalTokenUsage(transcriptData []byte, fromOffset int, subagentsDir string) (*agent.TokenUsage, error) {
	return CalculateTotalTokenUsageFromBytes(transcriptData, fromOffset, subagentsDir)
}

// --- Internal hook parsing functions ---

// resolveTranscriptPath computes the transcript path from session_id.
// Autohand stores transcripts at ~/.autohand/sessions/<session-id>/conversation.jsonl
func (a *AutohandCodeAgent) resolveTranscriptPath(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	sessionDir, err := a.GetSessionDir("")
	if err != nil {
		return ""
	}
	return filepath.Join(sessionDir, sessionID, "conversation.jsonl")
}

func (a *AutohandCodeAgent) parseSessionStart(stdin io.Reader) (*agent.Event, error) {
	raw, err := agent.ReadAndParseHookInput[hookInputBase](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.SessionStart,
		SessionID:  raw.SessionID,
		SessionRef: a.resolveTranscriptPath(raw.SessionID),
		Timestamp:  time.Now(),
	}, nil
}

func (a *AutohandCodeAgent) parseTurnStart(stdin io.Reader) (*agent.Event, error) {
	raw, err := agent.ReadAndParseHookInput[hookInputPrompt](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.TurnStart,
		SessionID:  raw.SessionID,
		SessionRef: a.resolveTranscriptPath(raw.SessionID),
		Prompt:     raw.Instruction,
		Timestamp:  time.Now(),
	}, nil
}

func (a *AutohandCodeAgent) parseTurnEnd(stdin io.Reader) (*agent.Event, error) {
	raw, err := agent.ReadAndParseHookInput[hookInputStop](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.TurnEnd,
		SessionID:  raw.SessionID,
		SessionRef: a.resolveTranscriptPath(raw.SessionID),
		Timestamp:  time.Now(),
	}, nil
}

func (a *AutohandCodeAgent) parseSessionEnd(stdin io.Reader) (*agent.Event, error) {
	raw, err := agent.ReadAndParseHookInput[hookInputBase](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.SessionEnd,
		SessionID:  raw.SessionID,
		SessionRef: a.resolveTranscriptPath(raw.SessionID),
		Timestamp:  time.Now(),
	}, nil
}

func (a *AutohandCodeAgent) parseSubagentEnd(stdin io.Reader) (*agent.Event, error) {
	raw, err := agent.ReadAndParseHookInput[hookInputSubagent](stdin)
	if err != nil {
		return nil, err
	}
	return &agent.Event{
		Type:       agent.SubagentEnd,
		SessionID:  raw.SessionID,
		SessionRef: a.resolveTranscriptPath(raw.SessionID),
		SubagentID: raw.SubagentID,
		Timestamp:  time.Now(),
	}, nil
}
