package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jefferson-julio/h77p/internal/envfile"
	"github.com/jefferson-julio/h77p/internal/executor"
	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/script"
)

type Result struct {
	Request  *httpfile.Request
	HTTP     *executor.Result
	Tests    []*script.TestResult
	Passed   bool
	Err      error
	Logs     []string
	JQOutput string // result of @jq filters applied to response body; empty if none/failed
}

// SeedEnv pre-populates vars with .env file contents and file-level @variable
// declarations. Useful for showing the variable state before any request runs.
func SeedEnv(file *httpfile.File, vars map[string]string) {
	seedEnvVars(file, vars)
	seedFileVars(file, vars)
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
	// Request-level variables override file-level and .env variables.
	for _, v := range req.Variables {
		vars[v.Name] = v.Value
	}

	// shallow copy so pre-script mutations don't modify the parsed file
	reqCopy := *req
	if len(req.Headers) > 0 {
		reqCopy.Headers = make([]httpfile.Header, len(req.Headers))
		copy(reqCopy.Headers, req.Headers)
	}
	req = &reqCopy

	eng := script.New()
	var logs []string

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
			return &Result{Request: req, Err: err, Logs: preCtx.Logs}, nil
		}
		logs = append(logs, preCtx.Logs...)
		req.Method = preCtx.Request.Method
		req.URL = preCtx.Request.URL
		req.Body = preCtx.Request.Body
		req.Headers = mapToHeaders(preCtx.Request.Headers)
	}

	httpResult, err := executor.Execute(req, vars)
	if err != nil {
		return &Result{Request: req, Err: err, Logs: logs}, nil
	}

	// Apply @jq filters to the response body when the response is JSON.
	var jqOutput string
	if len(req.JQFilters) > 0 {
		if isJSONResponse(httpResult) {
			var jqErr error
			jqOutput, jqErr = applyJQFilters(httpResult.Body, req.JQFilters)
			if jqErr != nil {
				logs = append(logs, "jq: "+jqErr.Error())
			}
		} else {
			logs = append(logs, "jq: skipped — response is not JSON")
		}
	}

	result := &Result{
		Request:  req,
		HTTP:     httpResult,
		Passed:   true,
		JQOutput: jqOutput,
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
				JQOutput:   jqOutput,
			},
			Env: vars,
		}
		tests, scriptErr := eng.RunPostResponse(req.PostScript, postCtx)
		logs = append(logs, postCtx.Logs...)
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

	result.Logs = logs
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

// isJSONResponse returns true when the HTTP response has a JSON Content-Type.
func isJSONResponse(r *executor.Result) bool {
	for _, vs := range r.Headers {
		for _, v := range vs {
			if strings.Contains(strings.ToLower(v), "application/json") {
				return true
			}
		}
	}
	return false
}

// applyJQFilters runs each filter through the jq binary in sequence, piping
// the output of one filter as the input of the next. Returns the final output.
func applyJQFilters(body string, filters []string) (string, error) {
	if _, err := exec.LookPath("jq"); err != nil {
		return "", fmt.Errorf("jq not found in PATH")
	}
	current := body
	for _, filter := range filters {
		cmd := exec.Command("jq", filter)
		cmd.Stdin = strings.NewReader(current)
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("filter %q: %w", filter, err)
		}
		current = strings.TrimRight(string(out), "\n")
	}
	return current, nil
}
