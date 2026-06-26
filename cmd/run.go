package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jefferson-julio/h77p/internal/parser"
	"github.com/jefferson-julio/h77p/internal/runner"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <file> [request-name]",
	Short: "Execute a request from a .http file",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
		var requestName string
		if len(args) > 1 {
			requestName = args[1]
		}

		f, err := parser.ParseFile(filePath)
		if err != nil {
			return err
		}

		result, err := runner.Run(f, requestName, make(map[string]string))
		if err != nil {
			return err
		}
		if result.Err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", result.Err)
			os.Exit(1)
		}

		h := result.HTTP
		fmt.Printf("%s %s\n\n", result.Request.Method, h.FinalURL)
		fmt.Printf("%s  %dms\n", h.Status, h.Duration.Milliseconds())

		// Print response headers sorted for stable output.
		keys := make([]string, 0, len(h.Headers))
		for k := range h.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s: %s\n", k, strings.Join(h.Headers[k], ", "))
		}

		if h.Body != "" {
			fmt.Printf("\n%s\n", h.Body)
		}

		// Print test results if the request had a post-script.
		if len(result.Tests) > 0 {
			fmt.Println()
			for _, t := range result.Tests {
				if t.Passed {
					fmt.Printf("  PASS  %s\n", t.Name)
				} else {
					fmt.Printf("  FAIL  %s — %s\n", t.Name, t.Error)
				}
			}
		}

		return nil
	},
}
