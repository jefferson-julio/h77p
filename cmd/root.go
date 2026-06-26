package cmd

import (
	"os"

	"github.com/jefferson-julio/h77p/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "h77p",
	Short: "A terminal HTTP client driven by .http files",
	// Running h77p with no subcommand launches the TUI at the current directory.
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return ui.Start(cwd)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(uiCmd)
}
