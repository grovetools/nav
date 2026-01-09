package main

import (
	"context"
	"encoding/json"
	"fmt"

	grovelogging "github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/version"
	"github.com/spf13/cobra"
)

var ulogVersion = grovelogging.NewUnifiedLogger("gmux.version")

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information for this binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		jsonOutput, _ := cmd.Flags().GetBool("json")
		info := version.GetInfo()

		if jsonOutput {
			jsonData, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal version info to JSON: %w", err)
			}
			ulogVersion.Info("Version output").
				Field("format", "json").
				Field("version", info.Version).
				Pretty(string(jsonData)).
				PrettyOnly().
				Log(ctx)
		} else {
			ulogVersion.Info("Version output").
				Field("format", "text").
				Field("version", info.Version).
				Pretty(info.String()).
				PrettyOnly().
				Log(ctx)
		}
		return nil
	},
}

func init() {
	versionCmd.Flags().Bool("json", false, "Output version information in JSON format")
}