package runner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jefferson-julio/h77p/internal/envfile"
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

// Run executes a single named request from file. If requestName is empty the
// first request is used.
func Run(file *httpfile.File, requestName string, vars map[string]string) (*Result, error) {
	seedEnvVars(file, vars)
	seedFileVars(file, vars)

	if requestName == "" {
		if len(file.Requests) == 0 {
			return nil, fmt.Errorf("no requests in file")
		}
		return runOne(&file.Requests[0], vars)
	}

	for i := range file.Requests {
		if file.Requests[i].Name == requestName {
			return runOne(&file.Requests[i], vars)
		}
	}
	return nil, fmt.Errorf("request %q not found", requestName)
}

// RunAll executes every request in file sequentially. Variables written by
// set() in one post-script are visible to subsequent requests.
func RunAll(file *httpfile.File, vars map[string]string) ([]*Result, error) {
	seedEnvVars(file, vars)
	seedFileVars(file, vars)

	results := make([]*Result, 0, len(file.Requests))
	for i := range file.Requests {
		r, err := runOne(&file.Requests[i], vars)
		if err != nil {
			return results, err
		}
		results = append(results, r)
		if r.Err != nil {
			return results, r.Err
		}
	}
	return results, nil
}

func runOne(req *httpfile.Request, vars map[string]string) (*Result, error) {
	// shallow copy so pre-script mutations don't modify the parsed file
	reqCopy := *req
	if len(req.Headers) > 0 {
		reqCopy.Headers = make([]httpfile.Header, len(req.Headers))
		copy(reqCopy.Headers, req.Headers)
	}
	req = &reqCopy

	eng := script.New()

	if req.PreScript != "" {
		preCtx := &script.PreContext{
			Request: &script.ScriptRequest{
				Method:  req.Method,
				URL:     req.URL,
				Headers: headersToMap(req.Headers),
				Body:    req.Body,
			},
			Env: vars,
		}
		if err := eng.RunPreRequest(req.PreScript, preCtx); err != nil {
			return &Result{Request: req, Err: err}, nil
		}
		req.Method = preCtx.Request.Method
		req.URL = preCtx.Request.URL
		req.Body = preCtx.Request.Body
		req.Headers = mapToHeaders(preCtx.Request.Headers)
	}

	httpResult, err := executor.Execute(req, vars)
	if err != nil {
		return &Result{Request: req, Err: err}, nil
	}

	result := &Result{
		Request: req,
		HTTP:    httpResult,
		Passed:  true,
	}

	if req.PostScript != "" {
		postCtx := &script.PostContext{
			Request: &script.ScriptRequest{
				Method:  req.Method,
				URL:     req.URL,
				Headers: headersToMap(req.Headers),
				Body:    req.Body,
			},
			Response: &script.ScriptResponse{
				Status:     httpResult.StatusCode,
				StatusText: httpResult.Status,
				Headers:    flattenHeaders(httpResult.Headers),
				Body:       httpResult.Body,
				Duration:   httpResult.Duration.Milliseconds(),
			},
			Env: vars,
		}
		tests, scriptErr := eng.RunPostResponse(req.PostScript, postCtx)
		result.Tests = tests
		if scriptErr != nil {
			result.Err = scriptErr
			result.Passed = false
		}
		for _, t := range tests {
			if !t.Passed {
				result.Passed = false
			}
		}
	}

	return result, nil
}

func headersToMap(headers []httpfile.Header) map[string]string {
	m := make(map[string]string, len(headers))
	for _, h := range headers {
		m[h.Name] = h.Value
	}
	return m
}

func mapToHeaders(m map[string]string) []httpfile.Header {
	headers := make([]httpfile.Header, 0, len(m))
	for k, v := range m {
		headers = append(headers, httpfile.Header{Name: k, Value: v})
	}
	return headers
}

func flattenHeaders(headers map[string][]string) map[string]string {
	m := make(map[string]string, len(headers))
	for k, vs := range headers {
		if len(vs) > 0 {
			m[k] = vs[0]
		}
	}
	return m
}

// seedEnvVars loads .env files from the http file's directory up to the process
// CWD and adds their variables to vars (lowest priority — only sets if absent).
func seedEnvVars(file *httpfile.File, vars map[string]string) {
	if file.Path == "" {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	for k, v := range envfile.Load(filepath.Dir(file.Path), cwd) {
		if _, exists := vars[k]; !exists {
			vars[k] = v
		}
	}
}

// seedFileVars copies file-level @variable declarations into vars. HTTP file
// variables take precedence over .env values, so they always overwrite.
func seedFileVars(file *httpfile.File, vars map[string]string) {
	for _, v := range file.Variables {
		vars[v.Name] = v.Value
	}
}
