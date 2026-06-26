package cmd

import (
	"os"

	"github.com/jefferson-julio/h77p/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "h77p [file.http]",
	Short: "A terminal HTTP client driven by .http files",
	Args:  cobra.MaximumNArgs(1),
	// Running h77p with no args launches the TUI browser at the current directory.
	// Running h77p <file.http> opens that file directly in the TUI file view.
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return ui.StartAtFile(args[0])
		}
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
