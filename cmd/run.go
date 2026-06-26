package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <file> [request-name]",
	Short: "Execute a request from a .http file",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("not implemented")
		return nil
	},
}
