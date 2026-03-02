package autohandcode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallHooks_FreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}
	count, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// 5 hooks: session-start, pre-prompt, stop, session-end, subagent-stop
	if count != 5 {
		t.Errorf("InstallHooks() count = %d, want 5", count)
	}

	// Verify config.json was created with hooks
	config := readAutohandConfig(t, tempDir)

	if len(config.Hooks.Hooks) != 5 {
		t.Errorf("Hook count = %d, want 5", len(config.Hooks.Hooks))
	}

	// Verify hook events
	assertAutohandHookExists(t, config.Hooks.Hooks, "session-start", entireHookPrefix+"session-start", "session-start")
	assertAutohandHookExists(t, config.Hooks.Hooks, "pre-prompt", entireHookPrefix+"pre-prompt", "pre-prompt")
	assertAutohandHookExists(t, config.Hooks.Hooks, "stop", entireHookPrefix+"stop", "stop")
	assertAutohandHookExists(t, config.Hooks.Hooks, "session-end", entireHookPrefix+"session-end", "session-end")
	assertAutohandHookExists(t, config.Hooks.Hooks, "subagent-stop", entireHookPrefix+"subagent-stop", "subagent-stop")

	// Verify AreHooksInstalled returns true
	if !ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return true after install")
	}
}

func TestInstallHooks_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}

	// First install
	count1, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}
	if count1 != 5 {
		t.Errorf("first InstallHooks() count = %d, want 5", count1)
	}

	// Second install should add 0 hooks
	count2, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("second InstallHooks() error = %v", err)
	}
	if count2 != 0 {
		t.Errorf("second InstallHooks() count = %d, want 0 (idempotent)", count2)
	}

	// Verify still only 5 hooks total
	config := readAutohandConfig(t, tempDir)
	if len(config.Hooks.Hooks) != 5 {
		t.Errorf("Hook count = %d after double install, want 5", len(config.Hooks.Hooks))
	}
}

func TestInstallHooks_LocalDev(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}
	_, err := ag.InstallHooks(context.Background(), true, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	config := readAutohandConfig(t, tempDir)

	// Verify local dev commands use AUTOHAND_PROJECT_DIR format
	assertAutohandHookExists(t, config.Hooks.Hooks, "session-start",
		entireHookLocalDevPrefix+"session-start", "session-start localDev")
	assertAutohandHookExists(t, config.Hooks.Hooks, "stop",
		entireHookLocalDevPrefix+"stop", "stop localDev")
	assertAutohandHookExists(t, config.Hooks.Hooks, "pre-prompt",
		entireHookLocalDevPrefix+"pre-prompt", "pre-prompt localDev")
	assertAutohandHookExists(t, config.Hooks.Hooks, "session-end",
		entireHookLocalDevPrefix+"session-end", "session-end localDev")
	assertAutohandHookExists(t, config.Hooks.Hooks, "subagent-stop",
		entireHookLocalDevPrefix+"subagent-stop", "subagent-stop localDev")
}

func TestInstallHooks_Force(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}

	// First install
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}

	// Force reinstall should replace hooks
	count, err := ag.InstallHooks(context.Background(), false, true)
	if err != nil {
		t.Fatalf("force InstallHooks() error = %v", err)
	}
	if count != 5 {
		t.Errorf("force InstallHooks() count = %d, want 5", count)
	}
}

func TestInstallHooks_PermissionsDeny_FreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	rules := readAutohandPermissionRules(t, tempDir)

	// Verify permissions.rules contains our deny rule
	if !containsDenyRule(rules, metadataDenyRule) {
		t.Errorf("permissions.rules = %v, want to contain deny rule for .entire/metadata/**", rules)
	}
}

func TestInstallHooks_PermissionsDeny_Idempotent(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}
	// First install
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("first InstallHooks() error = %v", err)
	}

	// Second install
	_, err = ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("second InstallHooks() error = %v", err)
	}

	rules := readAutohandPermissionRules(t, tempDir)

	// Count occurrences of our rule
	count := 0
	for _, rule := range rules {
		if rule == metadataDenyRule {
			count++
		}
	}
	if count != 1 {
		t.Errorf("permissions.rules contains %d copies of deny rule, want 1", count)
	}
}

func TestInstallHooks_PermissionsDeny_PreservesUserRules(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create config.json with existing user permission rule
	writeAutohandConfigFile(t, tempDir, `{
  "permissions": {
    "rules": [{"tool":"exec_command","pattern":"rm -rf *","action":"deny"}]
  }
}`)

	ag := &AutohandCodeAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	rules := readAutohandPermissionRules(t, tempDir)

	// Verify both rules exist
	userRule := AutohandPermissionRule{Tool: "exec_command", Pattern: "rm -rf *", Action: "deny"}
	if !containsDenyRule(rules, userRule) {
		t.Errorf("permissions.rules = %v, want to contain user rule", rules)
	}
	if !containsDenyRule(rules, metadataDenyRule) {
		t.Errorf("permissions.rules = %v, want to contain Entire rule", rules)
	}
}

func TestInstallHooks_PreservesUnknownFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create config.json with unknown top-level fields
	writeAutohandConfigFile(t, tempDir, `{
  "model": "gpt-4",
  "customSetting": {"nested": "value"}
}`)

	ag := &AutohandCodeAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// Read raw config to check for unknown fields
	configPath := filepath.Join(tempDir, ".autohand", AutohandConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}

	// Verify "model" field is preserved
	if _, ok := rawConfig["model"]; !ok {
		t.Errorf("model field was not preserved")
	}

	// Verify "customSetting" field is preserved
	if _, ok := rawConfig["customSetting"]; !ok {
		t.Errorf("customSetting field was not preserved")
	}
}

//nolint:tparallel // Parent uses t.Chdir() which prevents t.Parallel(); subtests only read from pre-loaded data
func TestInstallHooks_PreservesUserHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create config with user hooks on the same events we use
	writeAutohandConfigFile(t, tempDir, `{
  "hooks": {
    "hooks": [
      {"event":"stop","command":"echo user stop hook","description":"User stop hook","enabled":true}
    ]
  }
}`)

	ag := &AutohandCodeAgent{}
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	config := readAutohandConfig(t, tempDir)

	t.Run("user hook preserved", func(t *testing.T) {
		t.Parallel()
		assertAutohandHookExists(t, config.Hooks.Hooks, "stop", "echo user stop hook", "user stop hook")
	})

	t.Run("entire hook added", func(t *testing.T) {
		t.Parallel()
		assertAutohandHookExists(t, config.Hooks.Hooks, "stop", entireHookPrefix+"stop", "entire stop hook")
	})
}

func TestUninstallHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}

	// First install
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// Verify hooks are installed
	if !ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should be installed before uninstall")
	}

	// Uninstall
	err = ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	// Verify hooks are removed
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("hooks should not be installed after uninstall")
	}
}

func TestUninstallHooks_NoConfigFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}

	// Should not error when no config file exists
	err := ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() should not error when no config file: %v", err)
	}
}

func TestUninstallHooks_PreservesUserHooks(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create config with both user and entire hooks
	writeAutohandConfigFile(t, tempDir, `{
  "hooks": {
    "hooks": [
      {"event":"stop","command":"echo user hook","description":"User hook","enabled":true},
      {"event":"stop","command":"entire hooks autohand-code stop","description":"Entire","enabled":true}
    ]
  }
}`)

	ag := &AutohandCodeAgent{}
	err := ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	config := readAutohandConfig(t, tempDir)

	// Verify only user hooks remain
	if len(config.Hooks.Hooks) != 1 {
		t.Errorf("Hook count = %d after uninstall, want 1 (user only)", len(config.Hooks.Hooks))
	}

	// Verify it is the user hook
	if len(config.Hooks.Hooks) > 0 {
		if config.Hooks.Hooks[0].Command != "echo user hook" {
			t.Error("user hook was removed during uninstall")
		}
	}
}

func TestUninstallHooks_RemovesDenyRule(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}

	// First install (which adds the deny rule)
	_, err := ag.InstallHooks(context.Background(), false, false)
	if err != nil {
		t.Fatalf("InstallHooks() error = %v", err)
	}

	// Verify deny rule was added
	rules := readAutohandPermissionRules(t, tempDir)
	if !containsDenyRule(rules, metadataDenyRule) {
		t.Fatal("deny rule should be present after install")
	}

	// Uninstall
	err = ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	// Verify deny rule was removed
	rules = readAutohandPermissionRules(t, tempDir)
	if containsDenyRule(rules, metadataDenyRule) {
		t.Error("deny rule should be removed after uninstall")
	}
}

func TestUninstallHooks_PreservesUserDenyRules(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create config with user deny rule and entire deny rule
	writeAutohandConfigFile(t, tempDir, `{
  "permissions": {
    "rules": [
      {"tool":"exec_command","pattern":"rm -rf *","action":"deny"},
      {"tool":"read_file","pattern":".entire/metadata/**","action":"deny"}
    ]
  },
  "hooks": {
    "hooks": [
      {"event":"stop","command":"entire hooks autohand-code stop","description":"Entire","enabled":true}
    ]
  }
}`)

	ag := &AutohandCodeAgent{}
	err := ag.UninstallHooks(context.Background())
	if err != nil {
		t.Fatalf("UninstallHooks() error = %v", err)
	}

	rules := readAutohandPermissionRules(t, tempDir)

	// Verify user deny rule is preserved
	userRule := AutohandPermissionRule{Tool: "exec_command", Pattern: "rm -rf *", Action: "deny"}
	if !containsDenyRule(rules, userRule) {
		t.Errorf("user deny rule was removed, got: %v", rules)
	}

	// Verify entire deny rule is removed
	if containsDenyRule(rules, metadataDenyRule) {
		t.Errorf("entire deny rule should be removed, got: %v", rules)
	}
}

func TestAreHooksInstalled_NotInstalled(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ag := &AutohandCodeAgent{}
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return false when no config exists")
	}
}

func TestAreHooksInstalled_EmptyConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeAutohandConfigFile(t, tempDir, `{}`)

	ag := &AutohandCodeAgent{}
	if ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return false with empty config")
	}
}

func TestAreHooksInstalled_Installed(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeAutohandConfigFile(t, tempDir, `{
  "hooks": {
    "hooks": [
      {"event":"stop","command":"entire hooks autohand-code stop","description":"Entire","enabled":true}
    ]
  }
}`)

	ag := &AutohandCodeAgent{}
	if !ag.AreHooksInstalled(context.Background()) {
		t.Error("AreHooksInstalled() should return true when stop hook is present")
	}
}

// --- Helper functions ---

func writeAutohandConfigFile(t *testing.T, tempDir, content string) {
	t.Helper()
	autohandDir := filepath.Join(tempDir, ".autohand")
	if err := os.MkdirAll(autohandDir, 0o755); err != nil {
		t.Fatalf("failed to create .autohand dir: %v", err)
	}
	configPath := filepath.Join(autohandDir, AutohandConfigFileName)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}
}

func readAutohandConfig(t *testing.T, tempDir string) AutohandConfig {
	t.Helper()
	configPath := filepath.Join(tempDir, ".autohand", AutohandConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var config AutohandConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}
	return config
}

func readAutohandPermissionRules(t *testing.T, tempDir string) []AutohandPermissionRule {
	t.Helper()
	configPath := filepath.Join(tempDir, ".autohand", AutohandConfigFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}

	permRaw, ok := rawConfig["permissions"]
	if !ok {
		return nil
	}

	var rawPermissions map[string]json.RawMessage
	if err := json.Unmarshal(permRaw, &rawPermissions); err != nil {
		t.Fatalf("failed to parse permissions: %v", err)
	}

	rulesRaw, ok := rawPermissions["rules"]
	if !ok {
		return nil
	}

	var rules []AutohandPermissionRule
	if err := json.Unmarshal(rulesRaw, &rules); err != nil {
		t.Fatalf("failed to parse rules: %v", err)
	}
	return rules
}

func containsDenyRule(rules []AutohandPermissionRule, target AutohandPermissionRule) bool {
	for _, rule := range rules {
		if rule == target {
			return true
		}
	}
	return false
}

func assertAutohandHookExists(t *testing.T, hooks []AutohandHookDef, event, command, description string) {
	t.Helper()
	for _, h := range hooks {
		if h.Event == event && h.Command == command {
			return
		}
	}
	t.Errorf("%s hook not found (event=%q, command=%q)", description, event, command)
}
