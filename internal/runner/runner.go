package runner

import (
	"github.com/jefferson-julio/h77p/internal/executor"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/script"
)

type Result struct {
	Request *httpfile.Request
	HTTP    *executor.Result
	Tests   []*script.TestResult
	Passed  bool
	Err     error
}

func Run(file *httpfile.File, requestName string, vars map[string]string) (*Result, error) {
	return nil, nil
}

func RunAll(file *httpfile.File, vars map[string]string) ([]*Result, error) {
	return nil, nil
}
