package main

import (
	"context"

	"github.com/grovetools/core/pkg/mux"
	tmuxclient "github.com/grovetools/core/pkg/tmux"

	"github.com/grovetools/nav/pkg/tui/keymanage"
	"github.com/grovetools/nav/pkg/tui/sessionizer"
	"github.com/grovetools/nav/pkg/tui/windows"
)

// TuimuxDriver wraps a mux.MuxTUIEngine and satisfies the sessionizer
// and keymanage driver ports. It is wired in when ActiveMux() == MuxTuimux.
type TuimuxDriver struct {
	engine mux.MuxTUIEngine
}

func NewTuimuxDriver(engine mux.MuxTUIEngine) *TuimuxDriver {
	return &TuimuxDriver{engine: engine}
}

var (
	_ sessionizer.SessionDriver        = (*TuimuxDriver)(nil)
	_ sessionizer.SessionStateProvider = (*TuimuxDriver)(nil)
	_ keymanage.SessionDriver          = (*TuimuxDriver)(nil)
)

func (d *TuimuxDriver) Launch(ctx context.Context, sessionName, workingDir string) error {
	return d.engine.CreateSession(ctx, sessionName, mux.WithWorkDir(workingDir))
}

func (d *TuimuxDriver) SwitchTo(ctx context.Context, sessionName string) error {
	return d.engine.SwitchSession(ctx, sessionName)
}

func (d *TuimuxDriver) Kill(ctx context.Context, sessionName string) error {
	return d.engine.KillSession(ctx, sessionName)
}

func (d *TuimuxDriver) ClosePopup() error {
	return d.engine.ClosePopup(context.Background())
}

func (d *TuimuxDriver) ListActive(ctx context.Context) ([]string, error) {
	sessions, err := d.engine.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(sessions))
	for i, s := range sessions {
		names[i] = s.Name
	}
	return names, nil
}

func (d *TuimuxDriver) Exists(ctx context.Context, sessionName string) (bool, error) {
	return d.engine.SessionExists(ctx, sessionName)
}

// TuimuxWindowsDriver adapts a mux.MuxEngine to the windows.SessionDriver
// interface. Window-level operations (rename, move) are not yet supported
// in tuimux and return ErrNotImplemented.
type TuimuxWindowsDriver struct {
	engine mux.MuxEngine
}

func newTuimuxWindowsDriver(engine mux.MuxEngine) *TuimuxWindowsDriver {
	return &TuimuxWindowsDriver{engine: engine}
}

var _ windows.SessionDriver = (*TuimuxWindowsDriver)(nil)

func (d *TuimuxWindowsDriver) ListWindows(_ context.Context, _ string) ([]tmuxclient.Window, error) {
	return nil, mux.ErrNotImplemented
}

func (d *TuimuxWindowsDriver) CapturePane(ctx context.Context, target string) (string, error) {
	return d.engine.CapturePane(ctx, target)
}

func (d *TuimuxWindowsDriver) KillWindow(_ context.Context, _ string) error {
	return mux.ErrNotImplemented
}

func (d *TuimuxWindowsDriver) RenameWindow(_ context.Context, _, _ string) error {
	return mux.ErrNotImplemented
}

func (d *TuimuxWindowsDriver) MoveWindow(_ context.Context, _, _ string) error {
	return mux.ErrNotImplemented
}
