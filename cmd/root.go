package cmd

import (
	"fmt"
	"os"

	"github.com/jefferson-julio/h77p/internal/executor"
	"github.com/jefferson-julio/h77p/internal/ipc"
	"github.com/jefferson-julio/h77p/internal/ui"
	"github.com/spf13/cobra"
)

var themeName string
var socketPath string
var layoutName string
var maxBodyStr string

var rootCmd = &cobra.Command{
	Use:   "h77p [file.http]",
	Short: "A terminal HTTP client driven by .http files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if t, ok := ui.ThemeByName(themeName); ok {
			ui.InitTheme(t)
		}
		if l, ok := ui.LayoutByName(layoutName); ok {
			ui.InitLayout(l)
		}
		if maxBodyStr != "" {
			if n, err := executor.ParseBodySize(maxBodyStr); err == nil {
				executor.SetMaxBodySize(n)
			}
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
	rootCmd.PersistentFlags().StringVar(&layoutName, "layout", "auto", "panel layout: auto, horizontal (left/right), vertical (top/bottom)")
	rootCmd.PersistentFlags().StringVar(&maxBodyStr, "max-body", "1MB", "max response body size before spilling to disk (e.g. 512KB, 5MB, 10485760)")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(uiCmd)
}
