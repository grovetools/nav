package main

import (
	"fmt"

	"github.com/grovetools/core/pkg/mux"
	"github.com/spf13/cobra"

	"github.com/grovetools/nav/pkg/tmux"
)

// keyManageCmd is the cobra entry point for `nav key manage`. The
// actual model/view/update logic lives in nav/pkg/tui/keymanage. This
// shim just boots the unified nav TUI into the manage view — matching
// the pre-extraction behavior.
var keyManageCmd = &cobra.Command{
	Use:     "manage",
	Aliases: []string{"m"},
	Short:   "Interactively manage tmux session key mappings",
	Long:    `Open an interactive table to map/unmap sessions to keys. Use arrow keys to navigate, 'e' to map CWD to an empty key, and space to unmap. Changes are auto-saved on exit.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNavTUI()
	},
}

// reloadTmuxConfig reloads the tmux configuration. Used by the extracted
// nav TUI packages via Config.ReloadConfig.
func reloadTmuxConfig() error {
	if mux.ActiveMux() == mux.MuxNone {
		return fmt.Errorf("not in a tmux session")
	}
	cmd := tmux.Command("source-file", expandPath("~/.tmux.conf"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux reload failed: %s", string(output))
	}
	return nil
}

func init() {
	keyCmd.AddCommand(keyManageCmd)
}
