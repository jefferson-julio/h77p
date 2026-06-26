package executor

import (
	"time"

	"github.com/jefferson-julio/h77p/internal/httpfile"
)

type Result struct {
	Request    *httpfile.Request
	StatusCode int
	Status     string
	Headers    map[string][]string
	Body       string
	Duration   time.Duration
}

func Execute(req *httpfile.Request, vars map[string]string) (*Result, error) {
	return nil, nil
}
