package cmd

import (
	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui [file]",
	Short: "Launch the interactive TUI",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
