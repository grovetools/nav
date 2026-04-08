package bindings

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/models"
)

// GroupConfig holds the static config for a group needed for validation.
type GroupConfig struct {
	Prefix string
}

// Validate checks a NavSessionsFile for consistency errors.
// groupConfigs maps group name to its static configuration (prefix).
// Rules enforced:
//  1. No duplicate keys within a group.
//  2. Paths must be absolute or start with ~.
//  3. No workspace key in any group may conflict with another group's prefix key.
//
// Prefer ValidateAgainstPrevious when you have a prior on-disk state — it
// tolerates pre-existing rule-3 violations so users can still edit a file
// that was persisted before the validator was tightened.
func Validate(file *models.NavSessionsFile, groupConfigs map[string]GroupConfig) error {
	return ValidateAgainstPrevious(nil, file, groupConfigs)
}

// ValidateAgainstPrevious runs the same consistency rules as Validate, but
// suppresses rule-3 (prefix-trigger conflict) errors for conflicts that were
// already present in prev. Rules 1 and 2 always apply to newFile regardless
// of prev.
//
// This exists so a stale sessions.yml (or user config) already in violation
// does not permanently block all future writes through the daemon: the user
// must be able to edit the file in order to fix the violation. Only NEW
// rule-3 conflicts introduced by this write are rejected.
//
// Passing prev == nil is equivalent to strict mode (the old Validate).
func ValidateAgainstPrevious(prev, newFile *models.NavSessionsFile, groupConfigs map[string]GroupConfig) error {
	if newFile == nil {
		return nil
	}

	newGroups := groupEntries(newFile)

	// Rule 1: No empty keys (map semantics already enforce uniqueness).
	for _, g := range newGroups {
		for key := range g.sessions {
			if key == "" {
				return fmt.Errorf("group %q: empty key is not allowed", g.name)
			}
		}
	}

	// Rule 2: Paths must be absolute or ~-prefixed.
	for _, g := range newGroups {
		for key, sess := range g.sessions {
			if sess.Path == "" {
				continue // Empty path means unbound key slot.
			}
			if !filepath.IsAbs(sess.Path) && !strings.HasPrefix(sess.Path, "~") {
				return fmt.Errorf("group %q key %q: path %q must be absolute or start with ~", g.name, key, sess.Path)
			}
		}
	}

	// Rule 3: Prefix conflict detection — diff-aware against prev.
	triggerKeys := buildTriggerKeys(groupConfigs)
	newConflicts := collectPrefixConflicts(newGroups, triggerKeys)

	var prevConflicts map[string]string
	if prev != nil {
		prevConflicts = collectPrefixConflicts(groupEntries(prev), triggerKeys)
	}

	for conflictID, msg := range newConflicts {
		if _, preexisting := prevConflicts[conflictID]; preexisting {
			continue // Pre-existing violation — tolerate.
		}
		return fmt.Errorf("%s", msg)
	}

	return nil
}

// groupEntry bundles a group name with its sessions map for iteration.
type groupEntry struct {
	name     string
	sessions map[string]models.NavSessionConfig
}

// groupEntries flattens a NavSessionsFile into a slice of (name, sessions)
// entries, with "default" first.
func groupEntries(file *models.NavSessionsFile) []groupEntry {
	entries := make([]groupEntry, 0, 1+len(file.Groups))
	entries = append(entries, groupEntry{name: "default", sessions: file.Sessions})
	for name, gs := range file.Groups {
		entries = append(entries, groupEntry{name: name, sessions: gs.Sessions})
	}
	return entries
}

// buildTriggerKeys maps a group's extracted trigger key back to the group
// that owns it. Groups whose prefix has no trigger key (e.g. "<prefix>") are
// skipped.
func buildTriggerKeys(groupConfigs map[string]GroupConfig) map[string]string {
	triggerKeys := make(map[string]string, len(groupConfigs))
	for name, cfg := range groupConfigs {
		if tk := extractTriggerKey(cfg.Prefix); tk != "" {
			triggerKeys[tk] = name
		}
	}
	return triggerKeys
}

// collectPrefixConflicts returns every rule-3 conflict in the given groups,
// keyed by a stable identifier ("<group>:<key>") so two calls over prev and
// new states can be diffed. The value is the human-readable error string the
// caller will surface if the conflict is new.
func collectPrefixConflicts(groups []groupEntry, triggerKeys map[string]string) map[string]string {
	conflicts := make(map[string]string)
	for _, g := range groups {
		for key := range g.sessions {
			ownerGroup, isTrigger := triggerKeys[key]
			if !isTrigger || ownerGroup == g.name {
				continue
			}
			id := g.name + ":" + key
			conflicts[id] = fmt.Sprintf(
				"group %q key %q conflicts with group %q prefix trigger key",
				g.name, key, ownerGroup,
			)
		}
	}
	return conflicts
}

// extractTriggerKey returns the final key component from a prefix string.
// Examples: "<prefix> k" → "k", "<grove> k" → "k", "C-g" → "C-g", "<prefix>" → "", "" → ""
func extractTriggerKey(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "<prefix>" || prefix == "<grove>" {
		return ""
	}
	// "<prefix> X" or "<grove> X" → X is the trigger key
	if strings.HasPrefix(prefix, "<prefix> ") {
		return strings.TrimSpace(prefix[len("<prefix> "):])
	}
	if strings.HasPrefix(prefix, "<grove> ") {
		return strings.TrimSpace(prefix[len("<grove> "):])
	}
	// Raw key like "C-g" — this is the trigger key itself
	return prefix
}
