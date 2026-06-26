package cmd

import (
	"os"
	"path/filepath"

	"github.com/jefferson-julio/h77p/internal/ui"
	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui [file]",
	Short: "Launch the interactive TUI",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			abs, err := filepath.Abs(args[0])
			if err != nil {
				return err
			}
			return ui.StartAtFile(abs)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return ui.Start(cwd)
	},
}
