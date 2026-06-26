package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "h77p",
	Short: "A terminal HTTP client driven by .http files",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(uiCmd)
}
