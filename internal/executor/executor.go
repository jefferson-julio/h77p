package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jefferson-julio/h77p/internal/httpfile"
)

var varRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

type Result struct {
	Request    *httpfile.Request
	FinalURL   string
	Proto      string // e.g. "HTTP/1.1"
	StatusCode int
	Status     string // e.g. "200 OK"
	Headers    map[string][]string
	Body       string
	Duration   time.Duration
}

func Execute(req *httpfile.Request, vars map[string]string) (*Result, error) {
	url := interpolate(req.URL, vars)
	body := interpolate(req.Body, vars)

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for _, h := range req.Headers {
		httpReq.Header.Set(interpolate(h.Name, vars), interpolate(h.Value, vars))
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute %s %s: %w", req.Method, url, err)
	}
	defer resp.Body.Close()
	duration := time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	headers := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = v
	}

	return &Result{
		Request:    req,
		FinalURL:   url,
		Proto:      resp.Proto,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
		Body:       string(respBody),
		Duration:   duration,
	}, nil
}

func interpolate(s string, vars map[string]string) string {
	return varRe.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := vars[key]; ok {
			return v
		}
		return m
	})
}
