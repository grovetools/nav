package sessionizer

import (
	"testing"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/nav/pkg/api"
)

// fakeKeymapStore is a minimal Store whose session set lives in `sessions`,
// standing in for the shared Manager's authoritative in-memory map. It records
// what the sessionizer writes back so tests can assert no clobbering occurred.
type fakeKeymapStore struct {
	sessions []models.TmuxSession
}

func (f *fakeKeymapStore) GetSessions() ([]models.TmuxSession, error) {
	// Return a copy so callers can't mutate our authoritative slice in place.
	out := make([]models.TmuxSession, len(f.sessions))
	copy(out, f.sessions)
	return out, nil
}

func (f *fakeKeymapStore) UpdateSessions(sessions []models.TmuxSession) error {
	f.sessions = sessions
	return nil
}

func (f *fakeKeymapStore) GetAvailableProjects() ([]api.Project, error) { return nil, nil }
func (f *fakeKeymapStore) GetAccessHistory() (*workspace.AccessHistory, error) {
	return nil, nil
}
func (f *fakeKeymapStore) RecordProjectAccess(string) error  { return nil }
func (f *fakeKeymapStore) GetGroups() []string               { return nil }
func (f *fakeKeymapStore) GetActiveGroup() string            { return "default" }
func (f *fakeKeymapStore) SetActiveGroup(string)             {}
func (f *fakeKeymapStore) FindGroupForPath(string) string    { return "" }
func (f *fakeKeymapStore) GetGroupIcon(string) string        { return "" }
func (f *fakeKeymapStore) GetDefaultIcon() string            { return "" }
func (f *fakeKeymapStore) SetLastAccessedGroup(string) error { return nil }
func (f *fakeKeymapStore) CreateGroup(string, string) error  { return nil }
func (f *fakeKeymapStore) TakeSnapshot()                     {}
func (f *fakeKeymapStore) Undo() error                       { return nil }
func (f *fakeKeymapStore) Redo() error                       { return nil }
func (f *fakeKeymapStore) RegenerateBindings() error         { return nil }
func (f *fakeKeymapStore) ReloadBindingsFromDaemon() error   { return nil }

func sessionFor(key, path string) models.TmuxSession {
	return models.TmuxSession{Key: key, Path: path}
}

// TestClearKeyMapping_DoesNotClobberOutOfBandMapping reproduces the durability
// bug: the sessionizer holds a stale m.sessions snapshot while a mapping is
// added elsewhere (the Key Manage tab). Clearing an unrelated key must NOT
// erase the out-of-band mapping. The fix re-reads the latest sessions from the
// store before mutating.
func TestClearKeyMapping_DoesNotClobberOutOfBandMapping(t *testing.T) {
	const (
		staleKey  = "a"
		stalePath = "/ws/alpha"
		freshKey  = "w" // mapped out-of-band, absent from the stale snapshot
		freshPath = "/ws/omega"
	)

	store := &fakeKeymapStore{sessions: []models.TmuxSession{
		sessionFor(staleKey, stalePath),
		sessionFor(freshKey, freshPath), // the authoritative store has it
	}}

	// The model's cached snapshot predates the out-of-band 'w' mapping.
	m := &Model{
		store:    store,
		sessions: []models.TmuxSession{sessionFor(staleKey, stalePath)},
		keyMap:   make(map[string]string),
	}

	// User clears the unrelated 'a' mapping from the sessionizer.
	m.clearKeyMapping(stalePath)

	got, _ := store.GetSessions()
	var sawFresh, sawStaleCleared bool
	for _, s := range got {
		if s.Key == freshKey && s.Path == freshPath {
			sawFresh = true
		}
		if s.Key == staleKey && s.Path == "" {
			sawStaleCleared = true
		}
	}
	if !sawFresh {
		t.Fatalf("out-of-band mapping %q→%q was clobbered; store=%+v", freshKey, freshPath, got)
	}
	if !sawStaleCleared {
		t.Fatalf("expected %q to be cleared; store=%+v", staleKey, got)
	}
}

// TestUpdateKeyMapping_DoesNotClobberOutOfBandMapping is the same guarantee for
// the add/remap path.
func TestUpdateKeyMapping_DoesNotClobberOutOfBandMapping(t *testing.T) {
	const (
		freshKey   = "w"
		freshPath  = "/ws/omega"
		newKey     = "b"
		newProject = "/ws/beta"
	)

	store := &fakeKeymapStore{sessions: []models.TmuxSession{
		sessionFor("b", ""),             // free slot
		sessionFor(freshKey, freshPath), // out-of-band mapping
	}}

	// Stale snapshot lacks the 'w'→omega mapping.
	m := &Model{
		store:    store,
		sessions: []models.TmuxSession{sessionFor("b", "")},
		keyMap:   make(map[string]string),
	}

	// User maps a new project to key 'b' from the sessionizer.
	m.updateKeyMapping(newProject, newKey)

	got, _ := store.GetSessions()
	var sawFresh, sawNew bool
	for _, s := range got {
		if s.Key == freshKey && s.Path == freshPath {
			sawFresh = true
		}
		if s.Key == newKey && s.Path == newProject {
			sawNew = true
		}
	}
	if !sawFresh {
		t.Fatalf("out-of-band mapping %q→%q was clobbered; store=%+v", freshKey, freshPath, got)
	}
	if !sawNew {
		t.Fatalf("expected new mapping %q→%q; store=%+v", newKey, newProject, got)
	}
}
