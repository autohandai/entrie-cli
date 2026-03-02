//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent/autohandcode"
)

// Use the real Autohand types to avoid schema drift.
type AutohandConfig = autohandcode.AutohandConfig

// TestSetupAutohandHooks_AddsAllRequiredHooks is a smoke test verifying that
// `entire enable --agent autohand-code` adds all required hooks to the correct file.
func TestSetupAutohandHooks_AddsAllRequiredHooks(t *testing.T) {
	t.Parallel()
	env := NewTestEnv(t)
	env.InitRepo()
	env.InitEntire()

	// Create initial commit (required for setup)
	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	// Run entire enable --agent autohand-code (non-interactive)
	output, err := env.RunCLIWithError("enable", "--agent", "autohand-code")
	if err != nil {
		t.Fatalf("enable autohand-code command failed: %v\nOutput: %s", err, output)
	}

	// Read the generated config.json
	config := readAutohandConfigFile(t, env)

	// Verify all hooks exist (5 total)
	expectedEvents := map[string]bool{
		"session-start": false,
		"pre-prompt":    false,
		"stop":          false,
		"session-end":   false,
		"subagent-stop": false,
	}
	for _, hook := range config.Hooks.Hooks {
		if _, ok := expectedEvents[hook.Event]; ok {
			if strings.Contains(hook.Command, "entire hooks autohand-code") {
				expectedEvents[hook.Event] = true
			}
		}
	}
	for event, found := range expectedEvents {
		if !found {
			t.Errorf("%s hook should exist", event)
		}
	}

	// Verify permissions.rules contains metadata deny rule
	configPath := filepath.Join(env.RepoDir, ".autohand", autohandcode.AutohandConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, ".entire/metadata/**") {
		t.Error("config.json should contain permission deny rule for .entire/metadata/**")
	}
}

// TestSetupAutohandHooks_PreservesExistingSettings verifies that
// enable autohand-code doesn't nuke existing settings or user-configured hooks.
func TestSetupAutohandHooks_PreservesExistingSettings(t *testing.T) {
	t.Parallel()
	env := NewTestEnv(t)
	env.InitRepo()
	env.InitEntire()

	env.WriteFile("README.md", "# Test")
	env.GitAdd("README.md")
	env.GitCommit("Initial commit")

	// Create existing config with custom fields and user hooks
	autohandDir := filepath.Join(env.RepoDir, ".autohand")
	if err := os.MkdirAll(autohandDir, 0o755); err != nil {
		t.Fatalf("failed to create .autohand dir: %v", err)
	}

	existingConfig := `{
  "customSetting": "should-be-preserved",
  "hooks": {
    "enabled": true,
    "hooks": [
      {"event": "stop", "command": "echo user-stop-hook", "description": "My custom hook"}
    ]
  }
}`
	configPath := filepath.Join(autohandDir, autohandcode.AutohandConfigFileName)
	if err := os.WriteFile(configPath, []byte(existingConfig), 0o644); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	// Run enable autohand-code
	output, err := env.RunCLIWithError("enable", "--agent", "autohand-code")
	if err != nil {
		t.Fatalf("enable autohand-code failed: %v\nOutput: %s", err, output)
	}

	// Verify custom setting is preserved
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}

	if rawConfig["customSetting"] != "should-be-preserved" {
		t.Error("customSetting should be preserved after enable autohand-code")
	}

	// Verify user hooks are preserved
	config := readAutohandConfigFile(t, env)

	foundUserHook := false
	for _, hook := range config.Hooks.Hooks {
		if hook.Command == "echo user-stop-hook" {
			foundUserHook = true
		}
	}
	if !foundUserHook {
		t.Error("existing user hook 'echo user-stop-hook' should be preserved")
	}

	// Our hooks should also be added
	foundSessionStart := false
	foundPrePrompt := false
	for _, hook := range config.Hooks.Hooks {
		if hook.Event == "session-start" && strings.Contains(hook.Command, "entire hooks autohand-code") {
			foundSessionStart = true
		}
		if hook.Event == "pre-prompt" && strings.Contains(hook.Command, "entire hooks autohand-code") {
			foundPrePrompt = true
		}
	}
	if !foundSessionStart {
		t.Error("session-start hook should be added")
	}
	if !foundPrePrompt {
		t.Error("pre-prompt hook should be added")
	}
}

// Helper functions

func readAutohandConfigFile(t *testing.T, env *TestEnv) AutohandConfig {
	t.Helper()
	configPath := filepath.Join(env.RepoDir, ".autohand", autohandcode.AutohandConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read %s at %s: %v", autohandcode.AutohandConfigFileName, configPath, err)
	}

	var config AutohandConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}
	return config
}
