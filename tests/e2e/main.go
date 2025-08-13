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
		GtmuxListScenario(),
		GtmuxStatusScenario(),
		
		// Tmux-specific scenarios (only run locally with tmux installed)
		GtmuxSessionExistsScenario(),
		GtmuxSessionKillScenario(),
		GtmuxSessionCaptureScenario(),
		GtmuxLaunchScenario(),
		GtmuxWaitScenario(),
		GtmuxStartScenario(),
	}

	if err := app.Execute(context.Background(), scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}