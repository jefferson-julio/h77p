package cmd

import (
	"fmt"
	"os"

	"github.com/jefferson-julio/h77p/internal/parser"
	"github.com/jefferson-julio/h77p/internal/runner"
	"github.com/spf13/cobra"
)

var testCmd = &cobra.Command{
	Use:   "test <file>",
	Short: "Run all requests with test blocks in a .http file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := parser.ParseFile(args[0])
		if err != nil {
			return err
		}

		results, err := runner.RunAll(f, make(map[string]string))
		if err != nil {
			return err
		}

		passed, failed := 0, 0
		for _, r := range results {
			if r.Err != nil {
				fmt.Fprintf(os.Stderr, "error running %q: %v\n", r.Request.Name, r.Err)
				continue
			}

			name := r.Request.Name
			if name == "" {
				name = r.Request.Method + " " + r.HTTP.FinalURL
			}

			if len(r.Tests) == 0 {
				continue
			}

			fmt.Printf("%s  %s  %dms\n", r.HTTP.Status, name, r.HTTP.Duration.Milliseconds())
			for _, t := range r.Tests {
				if t.Passed {
					fmt.Printf("  PASS  %s\n", t.Name)
					passed++
				} else {
					fmt.Printf("  FAIL  %s — %s\n", t.Name, t.Error)
					failed++
				}
			}
		}

		fmt.Printf("\n%d passed, %d failed\n", passed, failed)

		if failed > 0 {
			os.Exit(1)
		}
		return nil
	},
}
