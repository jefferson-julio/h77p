package cmd

import (
	"fmt"
	"os"

	"github.com/jefferson-julio/h77p/internal/ipc"
	"github.com/jefferson-julio/h77p/internal/ui"
	"github.com/spf13/cobra"
)

var themeName string
var socketPath string

var rootCmd = &cobra.Command{
	Use:   "h77p [file.http]",
	Short: "A terminal HTTP client driven by .http files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if t, ok := ui.ThemeByName(themeName); ok {
			ui.InitTheme(t)
		}
		var srv *ipc.Server
		if socketPath != "" {
			var err error
			srv, err = ipc.New(socketPath)
			if err != nil {
				return fmt.Errorf("ipc: %w", err)
			}
			defer srv.Close()
		}
		if len(args) == 1 {
			return ui.StartAtFile(args[0], srv)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return ui.Start(cwd, srv)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&themeName, "theme", "catppuccin", "colour theme: monokai, nord, catppuccin")
	rootCmd.PersistentFlags().StringVar(&socketPath, "socket", os.Getenv("H77P_SOCKET"), "Unix socket path for IPC (default: $H77P_SOCKET)")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(uiCmd)
}
