package keymanage_test

import (
	"testing"

	"github.com/grovetools/nav/pkg/tmux"
	"github.com/grovetools/nav/pkg/tui/keymanage"
)

// TestTmuxManagerSatisfiesStore is a compile-time check that the
// nav *tmux.Manager implements keymanage.Store. The test body does
// nothing — the assignment below is the assertion.
func TestTmuxManagerSatisfiesStore(t *testing.T) {
	var _ keymanage.Store = (*tmux.Manager)(nil)
}
