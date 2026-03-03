package autohandcode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/jsonutil"
	"github.com/entireio/cli/cmd/entire/cli/paths"
)

// Ensure AutohandCodeAgent implements HookSupport
var _ agent.HookSupport = (*AutohandCodeAgent)(nil)

// Autohand Code hook names - these become subcommands under `entire hooks autohand-code`
const (
	HookNameSessionStart = "session-start"
	HookNameSessionEnd   = "session-end"
	HookNameStop         = "stop"
	HookNamePrePrompt    = "pre-prompt"
	HookNameSubagentStop = "subagent-stop"
	HookNameNotification = "notification"
)

// AutohandConfigFileName is the config file used by Autohand Code.
const AutohandConfigFileName = "config.json"

// metadataDenyRule blocks Autohand from reading Entire session metadata
var metadataDenyRule = AutohandPermissionRule{
	Tool:    "read_file",
	Pattern: ".entire/metadata/**",
	Action:  "deny",
}

// entireHookPrefix identifies Entire hooks in the config
const entireHookPrefix = "entire hooks autohand-code "

// entireHookLocalDevPrefix identifies local dev Entire hooks
const entireHookLocalDevPrefix = "go run ${AUTOHAND_PROJECT_DIR}/cmd/entire/main.go hooks autohand-code "

// InstallHooks installs Autohand Code hooks in .autohand/config.json.
// If force is true, removes existing Entire hooks before installing.
// Returns the number of hooks installed.
func (a *AutohandCodeAgent) InstallHooks(ctx context.Context, localDev bool, force bool) (int, error) {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot, err = os.Getwd() //nolint:forbidigo // Intentional fallback when WorktreeRoot() fails (tests run outside git repos)
		if err != nil {
			return 0, fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	configPath := filepath.Join(repoRoot, ".autohand", AutohandConfigFileName)

	// Read existing config if it exists
	var rawConfig map[string]json.RawMessage
	existingData, readErr := os.ReadFile(configPath) //nolint:gosec // path is constructed from cwd + fixed path
	if readErr == nil {
		if err := json.Unmarshal(existingData, &rawConfig); err != nil {
			return 0, fmt.Errorf("failed to parse existing config.json: %w", err)
		}
	} else {
		rawConfig = make(map[string]json.RawMessage)
	}

	// Parse hooks section
	var hooksSettings AutohandHooksSettings
	if hooksRaw, ok := rawConfig["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &hooksSettings); err != nil {
			return 0, fmt.Errorf("failed to parse hooks in config.json: %w", err)
		}
	}

	// If force is true, remove all existing Entire hooks first
	if force {
		hooksSettings.Hooks = removeEntireHooks(hooksSettings.Hooks)
	}

	// Define hook commands
	var prefix string
	if localDev {
		prefix = entireHookLocalDevPrefix
	} else {
		prefix = entireHookPrefix
	}

	count := 0

	// Add hooks if they don't exist
	hookDefs := []struct {
		event string
		verb  string
		desc  string
	}{
		{event: "session-start", verb: "session-start", desc: "Entire: session start checkpoint"},
		{event: "pre-prompt", verb: "pre-prompt", desc: "Entire: capture user prompt"},
		{event: "stop", verb: "stop", desc: "Entire: save checkpoint on agent stop"},
		{event: "session-end", verb: "session-end", desc: "Entire: session end cleanup"},
		{event: "subagent-stop", verb: "subagent-stop", desc: "Entire: subagent checkpoint"},
	}

	for _, def := range hookDefs {
		cmd := prefix + def.verb
		if !hookCommandExists(hooksSettings.Hooks, cmd) {
			enabled := true
			hooksSettings.Hooks = append(hooksSettings.Hooks, AutohandHookDef{
				Event:       def.event,
				Command:     cmd,
				Description: def.desc,
				Enabled:     &enabled,
				Timeout:     10000, // 10s for checkpoint operations
			})
			count++
		}
	}

	// Add permissions deny rule if not present
	permissionsChanged := false
	var rawPermissions map[string]json.RawMessage
	if permRaw, ok := rawConfig["permissions"]; ok {
		if err := json.Unmarshal(permRaw, &rawPermissions); err != nil {
			return 0, fmt.Errorf("failed to parse permissions in config.json: %w", err)
		}
	}
	if rawPermissions == nil {
		rawPermissions = make(map[string]json.RawMessage)
	}

	var rules []AutohandPermissionRule
	if rulesRaw, ok := rawPermissions["rules"]; ok {
		if err := json.Unmarshal(rulesRaw, &rules); err != nil {
			return 0, fmt.Errorf("failed to parse permissions.rules in config.json: %w", err)
		}
	}
	if !permissionRuleExists(rules, metadataDenyRule) {
		rules = append(rules, metadataDenyRule)
		rulesJSON, err := json.Marshal(rules)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal permission rules: %w", err)
		}
		rawPermissions["rules"] = rulesJSON
		permissionsChanged = true
	}

	if count == 0 && !permissionsChanged {
		return 0, nil // All hooks and permissions already installed
	}

	// Marshal hooks back
	hooksJSON, err := json.Marshal(hooksSettings)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal hooks: %w", err)
	}
	rawConfig["hooks"] = hooksJSON

	// Marshal permissions back
	permJSON, err := json.Marshal(rawPermissions)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal permissions: %w", err)
	}
	rawConfig["permissions"] = permJSON

	// Write back to file
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return 0, fmt.Errorf("failed to create .autohand directory: %w", err)
	}

	output, err := jsonutil.MarshalIndentWithNewline(rawConfig, "", "  ")
	if err != nil {
		return 0, fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, output, 0o600); err != nil {
		return 0, fmt.Errorf("failed to write config.json: %w", err)
	}

	return count, nil
}

// UninstallHooks removes Entire hooks from Autohand Code config.
func (a *AutohandCodeAgent) UninstallHooks(ctx context.Context) error {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "." // Fallback to CWD if not in a git repo
	}
	configPath := filepath.Join(repoRoot, ".autohand", AutohandConfigFileName)
	data, err := os.ReadFile(configPath) //nolint:gosec // path is constructed from repo root + fixed path
	if err != nil {
		return nil //nolint:nilerr // No config file means nothing to uninstall
	}

	var rawConfig map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return fmt.Errorf("failed to parse config.json: %w", err)
	}

	// Parse and filter hooks
	var hooksSettings AutohandHooksSettings
	if hooksRaw, ok := rawConfig["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &hooksSettings); err != nil {
			return fmt.Errorf("failed to parse hooks: %w", err)
		}
	}

	hooksSettings.Hooks = removeEntireHooks(hooksSettings.Hooks)

	// Remove the metadata deny rule from permissions
	var rawPermissions map[string]json.RawMessage
	if permRaw, ok := rawConfig["permissions"]; ok {
		if err := json.Unmarshal(permRaw, &rawPermissions); err != nil {
			rawPermissions = nil
		}
	}

	if rawPermissions != nil {
		if rulesRaw, ok := rawPermissions["rules"]; ok {
			var rules []AutohandPermissionRule
			if err := json.Unmarshal(rulesRaw, &rules); err == nil {
				filteredRules := make([]AutohandPermissionRule, 0, len(rules))
				for _, rule := range rules {
					if rule != metadataDenyRule {
						filteredRules = append(filteredRules, rule)
					}
				}
				if len(filteredRules) > 0 {
					rulesJSON, err := json.Marshal(filteredRules)
					if err == nil {
						rawPermissions["rules"] = rulesJSON
					}
				} else {
					delete(rawPermissions, "rules")
				}
			}
		}
		if len(rawPermissions) > 0 {
			permJSON, err := json.Marshal(rawPermissions)
			if err == nil {
				rawConfig["permissions"] = permJSON
			}
		} else {
			delete(rawConfig, "permissions")
		}
	}

	// Marshal hooks back
	if len(hooksSettings.Hooks) > 0 || hooksSettings.Enabled != nil {
		hooksJSON, err := json.Marshal(hooksSettings)
		if err != nil {
			return fmt.Errorf("failed to marshal hooks: %w", err)
		}
		rawConfig["hooks"] = hooksJSON
	} else {
		delete(rawConfig, "hooks")
	}

	// Write back
	output, err := jsonutil.MarshalIndentWithNewline(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(configPath, output, 0o600); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}
	return nil
}

// AreHooksInstalled checks if Entire hooks are installed.
func (a *AutohandCodeAgent) AreHooksInstalled(ctx context.Context) bool {
	repoRoot, err := paths.WorktreeRoot(ctx)
	if err != nil {
		repoRoot = "." // Fallback to CWD if not in a git repo
	}
	configPath := filepath.Join(repoRoot, ".autohand", AutohandConfigFileName)
	data, err := os.ReadFile(configPath) //nolint:gosec // path is constructed from repo root + fixed path
	if err != nil {
		return false
	}

	var config AutohandConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}

	// Check for at least one of our hooks (new or old format)
	return hookCommandExists(config.Hooks.Hooks, entireHookPrefix+"stop") ||
		hookCommandExists(config.Hooks.Hooks, entireHookLocalDevPrefix+"stop")
}

// Helper functions

func hookCommandExists(hooks []AutohandHookDef, command string) bool {
	for _, hook := range hooks {
		if hook.Command == command {
			return true
		}
	}
	return false
}

func isEntireHook(command string) bool {
	return strings.HasPrefix(command, entireHookPrefix) ||
		strings.HasPrefix(command, entireHookLocalDevPrefix)
}

func removeEntireHooks(hooks []AutohandHookDef) []AutohandHookDef {
	result := make([]AutohandHookDef, 0, len(hooks))
	for _, hook := range hooks {
		if !isEntireHook(hook.Command) {
			result = append(result, hook)
		}
	}
	return result
}

func permissionRuleExists(rules []AutohandPermissionRule, target AutohandPermissionRule) bool {
	for _, rule := range rules {
		if rule == target {
			return true
		}
	}
	return false
}
