package main

import (
	"fmt"
	"os"

	"github.com/jefferson-julio/h77p/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
