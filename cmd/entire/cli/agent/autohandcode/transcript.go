package autohandcode

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/transcript"
)

// TranscriptLine is an alias to the shared transcript.Line type.
type TranscriptLine = transcript.Line

// ParseAutohandTranscript parses an Autohand JSONL file into normalized transcript.Line entries.
// Autohand messages have a direct role field, unlike Droid's envelope format.
// Non-message entries (system role, tool role) are filtered to keep only user/assistant.
func ParseAutohandTranscript(path string, startLine int) ([]transcript.Line, int, error) {
	file, err := os.Open(path) //nolint:gosec // path is a controlled transcript file path
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open transcript: %w", err)
	}
	defer func() { _ = file.Close() }()

	return parseAutohandTranscriptFromReader(file, startLine)
}

// ParseAutohandTranscriptFromBytes parses Autohand JSONL content from a byte slice.
// startLine skips the first N raw JSONL lines before parsing (0 = parse all).
// Returns parsed lines, total raw line count, and any error.
func ParseAutohandTranscriptFromBytes(content []byte, startLine int) ([]transcript.Line, int, error) {
	return parseAutohandTranscriptFromReader(bytes.NewReader(content), startLine)
}

func parseAutohandTranscriptFromReader(r io.Reader, startLine int) ([]transcript.Line, int, error) {
	reader := bufio.NewReader(r)
	var lines []transcript.Line
	totalLines := 0

	for {
		lineBytes, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, 0, fmt.Errorf("failed to read transcript: %w", err)
		}

		if len(lineBytes) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}

		if totalLines >= startLine {
			if line, ok := parseAutohandLine(lineBytes); ok {
				lines = append(lines, line)
			}
		}
		totalLines++

		if err == io.EOF {
			break
		}
	}

	return lines, totalLines, nil
}

// parseAutohandLine converts a single Autohand JSONL line into a normalized transcript.Line.
// Returns false if the line is not a user or assistant message.
func parseAutohandLine(lineBytes []byte) (transcript.Line, bool) {
	var msg autohandMessage
	if err := json.Unmarshal(lineBytes, &msg); err != nil {
		return transcript.Line{}, false
	}

	// Only process "user" and "assistant" messages — skip "tool", "system", etc.
	if msg.Role != transcript.TypeUser && msg.Role != transcript.TypeAssistant {
		return transcript.Line{}, false
	}

	// Build a normalized message for downstream consumers.
	// For Autohand, we store the full message object so that:
	// - ExtractModifiedFiles can access toolCalls
	// - ExtractUserContent can access the content field
	// - ExtractSummary can access the content field
	normalizedMsg, err := json.Marshal(msg)
	if err != nil {
		return transcript.Line{}, false
	}

	return transcript.Line{
		Type:    msg.Role,
		UUID:    msg.Timestamp, // Use timestamp as a pseudo-UUID (Autohand has no message IDs)
		Message: normalizedMsg,
	}, true
}

// ExtractModifiedFiles extracts files modified by tool calls from transcript.
// Autohand stores tool calls in a separate toolCalls array on assistant messages.
func ExtractModifiedFiles(lines []TranscriptLine) []string {
	fileSet := make(map[string]bool)
	var files []string

	for _, line := range lines {
		if line.Type != transcript.TypeAssistant {
			continue
		}

		var msg autohandMessage
		if err := json.Unmarshal(line.Message, &msg); err != nil {
			continue
		}

		for _, tc := range msg.ToolCalls {
			if !slices.Contains(FileModificationTools, tc.Name) {
				continue
			}

			var input autohandToolInput
			if err := json.Unmarshal(tc.Input, &input); err != nil {
				continue
			}

			file := input.Path
			if file == "" {
				file = input.File
			}

			if file != "" && !fileSet[file] {
				fileSet[file] = true
				files = append(files, file)
			}
		}
	}

	return files
}

// CalculateTokenUsage calculates token usage from an Autohand transcript.
// Autohand stores token metadata in the _meta field of assistant messages.
func CalculateTokenUsage(transcriptLines []TranscriptLine) *agent.TokenUsage {
	usage := &agent.TokenUsage{}

	for _, line := range transcriptLines {
		if line.Type != transcript.TypeAssistant {
			continue
		}

		var msg autohandMessage
		if err := json.Unmarshal(line.Message, &msg); err != nil {
			continue
		}

		if msg.Meta == nil {
			continue
		}

		// Extract token info from _meta
		if inputTokens, ok := extractIntFromMeta(msg.Meta, "input_tokens"); ok {
			usage.InputTokens += inputTokens
		}
		if outputTokens, ok := extractIntFromMeta(msg.Meta, "output_tokens"); ok {
			usage.OutputTokens += outputTokens
		}
		if cacheCreation, ok := extractIntFromMeta(msg.Meta, "cache_creation_input_tokens"); ok {
			usage.CacheCreationTokens += cacheCreation
		}
		if cacheRead, ok := extractIntFromMeta(msg.Meta, "cache_read_input_tokens"); ok {
			usage.CacheReadTokens += cacheRead
		}
		usage.APICallCount++
	}

	return usage
}

// extractIntFromMeta safely extracts an integer from a map[string]any.
func extractIntFromMeta(meta map[string]any, key string) (int, bool) {
	v, ok := meta[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

// CalculateTokenUsageFromFile calculates token usage from a transcript file.
func CalculateTokenUsageFromFile(path string, startLine int) (*agent.TokenUsage, error) {
	if path == "" {
		return &agent.TokenUsage{}, nil
	}

	lines, _, err := ParseAutohandTranscript(path, startLine)
	if err != nil {
		return nil, err
	}

	return CalculateTokenUsage(lines), nil
}

// CalculateTotalTokenUsageFromBytes calculates token usage from pre-loaded transcript bytes,
// including subagents.
func CalculateTotalTokenUsageFromBytes(data []byte, startLine int, subagentsDir string) (*agent.TokenUsage, error) {
	if len(data) == 0 {
		return &agent.TokenUsage{}, nil
	}

	parsed, _, err := ParseAutohandTranscriptFromBytes(data, startLine)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	mainUsage := CalculateTokenUsage(parsed)

	agentIDs := ExtractSpawnedAgentIDs(parsed)
	if len(agentIDs) > 0 && subagentsDir != "" {
		subagentUsage := &agent.TokenUsage{}
		for agentID := range agentIDs {
			agentPath := filepath.Join(subagentsDir, fmt.Sprintf("agent-%s.jsonl", agentID))
			agentUsage, err := CalculateTokenUsageFromFile(agentPath, 0)
			if err != nil {
				continue
			}
			subagentUsage.InputTokens += agentUsage.InputTokens
			subagentUsage.CacheCreationTokens += agentUsage.CacheCreationTokens
			subagentUsage.CacheReadTokens += agentUsage.CacheReadTokens
			subagentUsage.OutputTokens += agentUsage.OutputTokens
			subagentUsage.APICallCount += agentUsage.APICallCount
		}
		if subagentUsage.APICallCount > 0 {
			mainUsage.SubagentTokens = subagentUsage
		}
	}

	return mainUsage, nil
}

// ExtractAllModifiedFilesFromBytes extracts files modified by both the main agent and
// any subagents from pre-loaded transcript bytes.
func ExtractAllModifiedFilesFromBytes(data []byte, startLine int, subagentsDir string) ([]string, error) {
	if len(data) == 0 {
		return nil, nil
	}

	parsed, _, err := ParseAutohandTranscriptFromBytes(data, startLine)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	files := ExtractModifiedFiles(parsed)
	fileSet := make(map[string]bool, len(files))
	for _, f := range files {
		fileSet[f] = true
	}

	agentIDs := ExtractSpawnedAgentIDs(parsed)
	if subagentsDir == "" {
		return files, nil
	}
	for agentID := range agentIDs {
		agentPath := filepath.Join(subagentsDir, fmt.Sprintf("agent-%s.jsonl", agentID))
		agentLines, _, agentErr := ParseAutohandTranscript(agentPath, 0)
		if agentErr != nil {
			continue
		}
		for _, f := range ExtractModifiedFiles(agentLines) {
			if !fileSet[f] {
				fileSet[f] = true
				files = append(files, f)
			}
		}
	}

	return files, nil
}

// ExtractSpawnedAgentIDs extracts agent IDs from subagent-stop data in the transcript.
// Returns a map of agentID -> toolUseID for all spawned agents.
func ExtractSpawnedAgentIDs(transcriptLines []TranscriptLine) map[string]string {
	agentIDs := make(map[string]string)

	for _, line := range transcriptLines {
		if line.Type != transcript.TypeAssistant {
			continue
		}

		var msg autohandMessage
		if err := json.Unmarshal(line.Message, &msg); err != nil {
			continue
		}

		// Look for Task tool calls that spawned subagents
		for _, tc := range msg.ToolCalls {
			if tc.Name != "task" && tc.Name != "Task" {
				continue
			}
			// The tool call ID is used as the subagent reference
			if tc.ID != "" {
				agentIDs[tc.ID] = tc.ID
			}
		}
	}

	return agentIDs
}
