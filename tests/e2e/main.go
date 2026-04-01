package main

import (
	"context"
	"fmt"
	"os"

	"github.com/grovetools/tend/pkg/app"
	"github.com/grovetools/tend/pkg/harness"
)

func main() {
	scenarios := []*harness.Scenario{
		// Original scenarios
		NavListScenario(),
		NavStatusScenario(),

		// Tmux-specific scenarios (only run locally with tmux installed)
		NavSessionExistsScenario(),
		NavSessionKillScenario(),
		NavSessionCaptureScenario(),
		NavLaunchScenario(),
		NavWaitScenario(),
		NavStartScenario(),
		NavWindowsScenario(),
		NavWindowsActiveSelectionScenario(),
		NavWindowsMoveScenario(),

		// nav sz column refactoring tests
		NavSzColsDefaultViewScenario(),
		NavSzColsKeysMappedScenario(),
		NavSzColsContextAppliedScenario(),
		NavSzColsCombinedViewScenario(),
		NavSzColsFilterHidesCxScenario(),

		// Delta-aware workspace update tests
		NavDeltaUpdatesGitScenario(),
		NavDeltaUpdatesNotesScenario(),
		NavDeltaIgnoresUnknownPathScenario(),
	}

	if err := app.Execute(context.Background(), scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
