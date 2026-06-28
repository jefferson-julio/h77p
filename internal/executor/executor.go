package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/script"
)

// maxBodySize is the per-response body limit. Responses larger than this are
// spilled to a temp file; Body holds the notice and BodyPath holds the file.
var maxBodySize int64 = 1 << 20 // 1 MB default

func SetMaxBodySize(n int64) { maxBodySize = n }

// ParseBodySize parses a human-readable size string ("1MB", "512KB", "2GB",
// or a raw byte count) and returns the number of bytes.
func ParseBodySize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	units := []struct {
		suffix string
		mult   int64
	}{
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"B", 1},
	}
	for _, u := range units {
		if strings.HasSuffix(upper, u.suffix) {
			n, err := strconv.ParseInt(strings.TrimSpace(s[:len(s)-len(u.suffix)]), 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q: %w", s, err)
			}
			return n * u.mult, nil
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return n, nil
}

var varRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

type Result struct {
	Request    *httpfile.Request
	FinalURL   string
	Proto      string // e.g. "HTTP/1.1"
	StatusCode int
	Status     string // e.g. "200 OK"
	Headers    map[string][]string
	Body       string
	BodyPath   string // non-empty when body exceeded maxBodySize and was spilled to disk
	Duration   time.Duration

	// Outgoing request details (post variable/expression expansion) for display.
	SentHeaders     map[string]string // resolved header values (one string per header)
	SentBody        string            // resolved body; empty for multipart or empty bodies
	SentBodyOmitted bool              // true when body was multipart (binary, not shown)
}

func Execute(req *httpfile.Request, vars map[string]string) (*Result, error) {
	// Lazily create the inline expression evaluator — only when a ${{ token is
	// encountered for the first time, avoiding runtime allocation for plain requests.
	var eval *script.InlineEvaluator
	expand := func(s string) string {
		s = interpolate(s, vars)
		if !script.HasInlineExprs(s) {
			return s
		}
		if eval == nil {
			eval = script.NewInlineEvaluator(vars)
		}
		return eval.Eval(s)
	}

	url := expand(req.URL)
	if strings.HasPrefix(url, "/") {
		if host, ok := vars["host"]; ok && host != "" {
			url = strings.TrimRight(host, "/") + url
		}
	}
	body := expand(req.Body)

	var bodyReader io.Reader
	var multipartContentType string

	switch {
	case isMultipartFormData(req.Headers, vars):
		fileDir := "."
		if req.FilePath != "" {
			fileDir = filepath.Dir(req.FilePath)
		}
		data, ct, err := buildMultipartBody(body, fileDir)
		if err != nil {
			return nil, fmt.Errorf("multipart: %w", err)
		}
		bodyReader = bytes.NewReader(data)
		multipartContentType = ct

	case isFormURLEncoded(req.Headers, vars):
		// x-www-form-urlencoded bodies are often written one param per line for
		// readability; collapse newlines into a single query string before sending.
		bodyReader = strings.NewReader(collapseFormBody(body))

	case body != "":
		bodyReader = strings.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	for _, h := range req.Headers {
		httpReq.Header.Set(expand(h.Name), expand(h.Value))
	}
	// Override Content-Type with the generated multipart boundary value.
	// This must happen after the headers loop so it takes precedence.
	if multipartContentType != "" {
		httpReq.Header.Set("Content-Type", multipartContentType)
	}

	// Capture the outgoing request details (after full expansion) for display.
	sentHeaders := make(map[string]string, len(httpReq.Header))
	for k, vs := range httpReq.Header {
		sentHeaders[k] = strings.Join(vs, ", ")
	}
	var sentBody string
	var sentBodyOmitted bool
	if multipartContentType != "" {
		sentBodyOmitted = true
	} else if isFormURLEncoded(req.Headers, vars) {
		sentBody = collapseFormBody(body)
	} else {
		sentBody = body
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute %s %s: %w", req.Method, url, err)
	}
	defer resp.Body.Close()
	duration := time.Since(start)

	body, bodyPath, err := readBodyWithLimit(resp.Body, maxBodySize)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	headers := make(map[string][]string, len(resp.Header))
	for k, v := range resp.Header {
		headers[k] = v
	}

	return &Result{
		Request:         req,
		FinalURL:        url,
		Proto:           resp.Proto,
		StatusCode:      resp.StatusCode,
		Status:          resp.Status,
		Headers:         headers,
		Body:            body,
		BodyPath:        bodyPath,
		Duration:        duration,
		SentHeaders:     sentHeaders,
		SentBody:        sentBody,
		SentBodyOmitted: sentBodyOmitted,
	}, nil
}

// readBodyWithLimit reads r up to limit bytes. If the response fits within the
// limit the body is returned as a string with an empty path. When the limit is
// exceeded the full response is streamed to a temp file; the returned string is
// a human-readable notice and path is the temp file location.
func readBodyWithLimit(r io.Reader, limit int64) (body, path string, err error) {
	if limit <= 0 {
		data, err := io.ReadAll(r)
		return string(data), "", err
	}

	// Read limit+1 bytes to detect overflow without buffering the whole response.
	probe, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return "", "", err
	}
	if int64(len(probe)) <= limit {
		return string(probe), "", nil
	}

	// Response exceeds limit — spill everything to a temp file.
	f, err := os.CreateTemp("", "h77p-body-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(probe); err != nil {
		return "", "", err
	}
	// Stream the remainder directly to disk.
	total := int64(len(probe))
	n, err := io.Copy(f, r)
	if err != nil {
		return "", "", err
	}
	total += n

	notice := fmt.Sprintf("[body too large (%s), stored at %s]", FormatBodySize(total), f.Name())
	return notice, f.Name(), nil
}

func FormatBodySize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func isMultipartFormData(headers []httpfile.Header, vars map[string]string) bool {
	for _, h := range headers {
		if strings.EqualFold(h.Name, "content-type") {
			return strings.Contains(strings.ToLower(interpolate(h.Value, vars)), "multipart/form-data")
		}
	}
	return false
}

func isFormURLEncoded(headers []httpfile.Header, vars map[string]string) bool {
	for _, h := range headers {
		if strings.EqualFold(h.Name, "content-type") {
			return strings.Contains(strings.ToLower(interpolate(h.Value, vars)), "x-www-form-urlencoded")
		}
	}
	return false
}

// collapseFormBody joins lines of a multi-line URL-encoded body into a single
// query string. Each line is trimmed; blank lines are skipped.
func collapseFormBody(body string) string {
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			b.WriteString(line)
		}
	}
	return b.String()
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
