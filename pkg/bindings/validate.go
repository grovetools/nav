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
func Validate(file *models.NavSessionsFile, groupConfigs map[string]GroupConfig) error {
	if file == nil {
		return nil
	}

	// Collect all groups: "default" uses file.Sessions, named groups use file.Groups[name].Sessions.
	type groupEntry struct {
		name     string
		sessions map[string]models.NavSessionConfig
	}

	var groups []groupEntry
	groups = append(groups, groupEntry{name: "default", sessions: file.Sessions})
	for name, gs := range file.Groups {
		groups = append(groups, groupEntry{name: name, sessions: gs.Sessions})
	}

	// Rule 1: No duplicate keys within a group (maps enforce this, but check for empty keys).
	for _, g := range groups {
		for key := range g.sessions {
			if key == "" {
				return fmt.Errorf("group %q: empty key is not allowed", g.name)
			}
		}
	}

	// Rule 2: Paths must be absolute or ~-prefixed.
	for _, g := range groups {
		for key, sess := range g.sessions {
			if sess.Path == "" {
				continue // Empty path means unbound key slot.
			}
			if !filepath.IsAbs(sess.Path) && !strings.HasPrefix(sess.Path, "~") {
				return fmt.Errorf("group %q key %q: path %q must be absolute or start with ~", g.name, key, sess.Path)
			}
		}
	}

	// Rule 3: Prefix conflict detection.
	// A workspace key in any group must not match another group's trigger key.
	// The trigger key is extracted from the group's prefix string:
	//   "<prefix> k" → trigger key is "k"
	//   "<grove> k"  → trigger key is "k"
	//   "C-g"        → trigger key is "C-g"
	//   "<prefix>"   → no trigger key (binds directly to prefix table)
	//   ""           → no trigger key

	triggerKeys := make(map[string]string) // trigger key → group name
	for name, cfg := range groupConfigs {
		tk := extractTriggerKey(cfg.Prefix)
		if tk != "" {
			triggerKeys[tk] = name
		}
	}

	for _, g := range groups {
		for key := range g.sessions {
			if ownerGroup, conflict := triggerKeys[key]; conflict && ownerGroup != g.name {
				return fmt.Errorf(
					"group %q key %q conflicts with group %q prefix trigger key",
					g.name, key, ownerGroup,
				)
			}
		}
	}

	return nil
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
