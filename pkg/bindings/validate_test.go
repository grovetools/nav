package bindings

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/models"
)

// TestValidateAgainstPrevious_TolerePreExistingConflict is the regression test
// for the nav-keymap persistence bug: when sessions.yml already contains a
// rule-3 violation (e.g. the default group maps "w" to a path while another
// group uses "w" as its prefix trigger), the user must still be able to edit
// the file through the daemon. ValidateAgainstPrevious should suppress the
// pre-existing conflict and only flag conflicts that are new in newFile.
func TestValidateAgainstPrevious_TolerePreExistingConflict(t *testing.T) {
	groupConfigs := map[string]GroupConfig{
		"default":  {Prefix: "<prefix>"},
		"personal": {Prefix: "<prefix> w"}, // trigger key: w
	}

	prev := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"w": {Path: "/users/alice/old-project"}, // pre-existing violation
		},
	}
	newFile := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"w": {Path: "/users/alice/NEW-project"}, // edited path, same conflicting key
			"f": {Path: "/users/alice/fresh"},       // brand new binding
		},
	}

	if err := ValidateAgainstPrevious(prev, newFile, groupConfigs); err != nil {
		t.Fatalf("expected pre-existing rule-3 conflict to be tolerated, got: %v", err)
	}
}

// TestValidateAgainstPrevious_RejectsNewConflict ensures that a rule-3
// conflict introduced by the incoming write (not present in prev) is still
// rejected.
func TestValidateAgainstPrevious_RejectsNewConflict(t *testing.T) {
	groupConfigs := map[string]GroupConfig{
		"default":  {Prefix: "<prefix>"},
		"personal": {Prefix: "<prefix> w"},
	}

	prev := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"f": {Path: "/users/alice/fresh"},
		},
	}
	newFile := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"f": {Path: "/users/alice/fresh"},
			"w": {Path: "/users/alice/NEW-violation"}, // brand-new conflict
		},
	}

	err := ValidateAgainstPrevious(prev, newFile, groupConfigs)
	if err == nil {
		t.Fatal("expected new rule-3 conflict to be rejected")
	}
	if !strings.Contains(err.Error(), `key "w"`) {
		t.Fatalf("error does not mention the offending key: %v", err)
	}
}

// TestValidateAgainstPrevious_NilPrevIsStrict confirms passing prev=nil is
// equivalent to the original strict Validate — useful for ad-hoc validation
// contexts that have no prior state.
func TestValidateAgainstPrevious_NilPrevIsStrict(t *testing.T) {
	groupConfigs := map[string]GroupConfig{
		"default":  {Prefix: "<prefix>"},
		"personal": {Prefix: "<prefix> w"},
	}

	newFile := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"w": {Path: "/users/alice/violation"},
		},
	}

	if err := ValidateAgainstPrevious(nil, newFile, groupConfigs); err == nil {
		t.Fatal("expected strict mode (prev=nil) to reject rule-3 conflict")
	}
	if err := Validate(newFile, groupConfigs); err == nil {
		t.Fatal("expected Validate to behave identically to strict mode")
	}
}

// TestValidateAgainstPrevious_Rules1And2StillApply makes sure the diff-aware
// tolerance does not leak into rules 1 (empty keys) and 2 (relative paths).
// These are basic sanity checks that must apply to every write.
func TestValidateAgainstPrevious_Rules1And2StillApply(t *testing.T) {
	groupConfigs := map[string]GroupConfig{
		"default": {Prefix: "<prefix>"},
	}

	// Rule 2: relative path rejected even if prev had it too.
	prev := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"a": {Path: "relative/path"},
		},
	}
	newFile := &models.NavSessionsFile{
		Sessions: map[string]models.NavSessionConfig{
			"a": {Path: "still/relative"},
		},
	}
	if err := ValidateAgainstPrevious(prev, newFile, groupConfigs); err == nil {
		t.Fatal("expected rule 2 (relative path) to reject even with prev in same state")
	}
}
