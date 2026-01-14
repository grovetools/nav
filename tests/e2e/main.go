package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mattsolo1/grove-tend/pkg/app"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

func main() {
	scenarios := []*harness.Scenario{
		// Original scenarios
		GmuxListScenario(),
		GmuxStatusScenario(),

		// Tmux-specific scenarios (only run locally with tmux installed)
		GmuxSessionExistsScenario(),
		GmuxSessionKillScenario(),
		GmuxSessionCaptureScenario(),
		GmuxLaunchScenario(),
		GmuxWaitScenario(),
		GmuxStartScenario(),
		GmuxWindowsScenario(),
		GmuxWindowsActiveSelectionScenario(),
		GmuxWindowsMoveScenario(),

		// gmux sz column refactoring tests
		GmuxSzColsDefaultViewScenario(),
		GmuxSzColsKeysMappedScenario(),
		GmuxSzColsContextAppliedScenario(),
		GmuxSzColsCombinedViewScenario(),
		GmuxSzColsFilterHidesCxScenario(),
	}

	if err := app.Execute(context.Background(), scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
