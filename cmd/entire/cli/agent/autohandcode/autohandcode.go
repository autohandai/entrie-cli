// Package autohandcode implements the Agent interface for Autohand Code CLI.
package autohandcode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/agent/types"
	"github.com/entireio/cli/cmd/entire/cli/paths"
)

//nolint:gochecknoinits // Agent self-registration is the intended pattern
func init() {
	agent.Register(agent.AgentNameAutohandCode, NewAutohandCodeAgent)
}

// AutohandCodeAgent implements the agent.Agent interface for Autohand Code CLI.
//
//nolint:revive // AutohandCodeAgent is clearer than Agent in this context
type AutohandCodeAgent struct{}

// NewAutohandCodeAgent creates a new Autohand Code agent instance.
func NewAutohandCodeAgent() agent.Agent {
	return &AutohandCodeAgent{}
}

// Name returns the agent registry key.
func (a *AutohandCodeAgent) Name() types.AgentName { return agent.AgentNameAutohandCode }

// Type returns the agent type identifier.
func (a *AutohandCodeAgent) Type() types.AgentType { return agent.AgentTypeAutohandCode }

// Description returns a human-readable description.
func (a *AutohandCodeAgent) Description() string {
	return "Autohand Code - autonomous coding agent"
}

// IsPreview returns true as Autohand Code integration is in preview.
func (a *AutohandCodeAgent) IsPreview() bool { return true }

// ProtectedDirs returns directories that Autohand Code uses for config/state.
func (a *AutohandCodeAgent) ProtectedDirs() []string { return []string{".autohand"} }

// DetectPresence checks if Autohand Code is configured in the repository.
func (a *AutohandCodeAgent) DetectPresence(ctx context.Context) (bool, error) {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "."
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".autohand")); err == nil {
		return true, nil
	}
	return false, nil
}

// ReadTranscript reads the raw JSONL transcript bytes for a session.
func (a *AutohandCodeAgent) ReadTranscript(sessionRef string) ([]byte, error) {
	data, err := os.ReadFile(sessionRef) //nolint:gosec // Path comes from agent hook input
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}
	return data, nil
}

// ChunkTranscript splits a JSONL transcript at line boundaries.
func (a *AutohandCodeAgent) ChunkTranscript(_ context.Context, content []byte, maxSize int) ([][]byte, error) {
	chunks, err := agent.ChunkJSONL(content, maxSize)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk transcript: %w", err)
	}
	return chunks, nil
}

// ReassembleTranscript concatenates JSONL chunks with newlines.
func (a *AutohandCodeAgent) ReassembleTranscript(chunks [][]byte) ([]byte, error) {
	return agent.ReassembleJSONL(chunks), nil
}

// GetSessionID extracts the session ID from hook input.
func (a *AutohandCodeAgent) GetSessionID(input *agent.HookInput) string { return input.SessionID }

// GetSessionDir returns the directory where Autohand Code stores session transcripts.
// Path: ~/.autohand/sessions/
func (a *AutohandCodeAgent) GetSessionDir(_ string) (string, error) {
	if override := os.Getenv("ENTIRE_TEST_AUTOHAND_PROJECT_DIR"); override != "" {
		return override, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if envHome := os.Getenv("AUTOHAND_HOME"); envHome != "" {
		return filepath.Join(envHome, "sessions"), nil
	}
	return filepath.Join(homeDir, ".autohand", "sessions"), nil
}

// ResolveSessionFile returns the path to an Autohand Code session file.
// Autohand stores transcripts at ~/.autohand/sessions/<session-id>/conversation.jsonl
func (a *AutohandCodeAgent) ResolveSessionFile(sessionDir, agentSessionID string) string {
	return filepath.Join(sessionDir, agentSessionID, "conversation.jsonl")
}

// ReadSession reads a session from Autohand Code's storage (JSONL transcript file).
func (a *AutohandCodeAgent) ReadSession(input *agent.HookInput) (*agent.AgentSession, error) {
	if input.SessionRef == "" {
		return nil, errors.New("session reference (transcript path) is required")
	}

	data, err := os.ReadFile(input.SessionRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read transcript: %w", err)
	}

	lines, _, err := ParseAutohandTranscriptFromBytes(data, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse transcript: %w", err)
	}

	return &agent.AgentSession{
		SessionID:     input.SessionID,
		AgentName:     a.Name(),
		SessionRef:    input.SessionRef,
		StartTime:     time.Now(),
		NativeData:    data,
		ModifiedFiles: ExtractModifiedFiles(lines),
	}, nil
}

// WriteSession writes a session to Autohand Code's storage (JSONL transcript file).
func (a *AutohandCodeAgent) WriteSession(_ context.Context, session *agent.AgentSession) error {
	if session == nil {
		return errors.New("session is nil")
	}

	if session.AgentName != "" && session.AgentName != a.Name() {
		return fmt.Errorf("session belongs to agent %q, not %q", session.AgentName, a.Name())
	}

	if session.SessionRef == "" {
		return errors.New("session reference (transcript path) is required")
	}

	if len(session.NativeData) == 0 {
		return errors.New("session has no native data to write")
	}

	if err := os.MkdirAll(filepath.Dir(session.SessionRef), 0o750); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	if err := os.WriteFile(session.SessionRef, session.NativeData, 0o600); err != nil {
		return fmt.Errorf("failed to write transcript: %w", err)
	}

	return nil
}

// FormatResumeCommand returns the command to resume an Autohand Code session.
func (a *AutohandCodeAgent) FormatResumeCommand(sessionID string) string {
	return "autohand resume " + sessionID
}
