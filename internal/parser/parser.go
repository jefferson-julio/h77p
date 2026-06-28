package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jefferson-julio/h77p/internal/httpfile"
)

var httpMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true, "TRACE": true,
	"CONNECT": true,
}

// ParseFile reads a .http file from disk and parses it.
func ParseFile(path string) (*httpfile.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseString(string(data), path)
}

// ParseString parses a .http file from a string. path is stored in File.Path.
func ParseString(src, path string) (*httpfile.File, error) {
	// Normalise CRLF so all logic only sees LF.
	src = strings.ReplaceAll(src, "\r\n", "\n")
	p := &p{lines: strings.Split(src, "\n"), path: path}
	return p.parse()
}

// ---------------------------------------------------------------------------
// Internal parser
// ---------------------------------------------------------------------------

type p struct {
	lines []string
	pos   int
	path  string
}

func (p *p) eof() bool    { return p.pos >= len(p.lines) }
func (p *p) peek() string { if p.eof() { return "" }; return p.lines[p.pos] }
func (p *p) next() string {
	if p.eof() {
		return ""
	}
	l := p.lines[p.pos]
	p.pos++
	return l
}
func (p *p) skipBlank() {
	for !p.eof() && strings.TrimSpace(p.peek()) == "" {
		p.pos++
	}
}

func (p *p) parse() (*httpfile.File, error) {
	file := &httpfile.File{Path: p.path}

	// File-level declarations: everything before the first ### separator.
	for !p.eof() {
		line := strings.TrimSpace(p.peek())
		if strings.HasPrefix(line, "###") {
			break
		}
		p.pos++
		switch {
		case line == "":
		case strings.HasPrefix(line, "#"):  // comment — skip
		case strings.HasPrefix(line, "@import "): // top-level @import without a group name — ignored
		case strings.HasPrefix(line, "@"):
			if v, err := parseVarDecl(line); err == nil {
				file.Variables = append(file.Variables, v)
			}
		}
	}

	// Request blocks: each starts with a ### separator line.
	for !p.eof() {
		line := strings.TrimSpace(p.next())
		if !strings.HasPrefix(line, "###") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(line, "###"))

		// Check whether this block is a group import: ### Name followed by @import path.
		if importPath, ok := p.peekImport(); ok {
			p.consumeImport()
			g := httpfile.Group{Name: name, Source: importPath}
			if p.path != "" {
				absPath := filepath.Join(filepath.Dir(p.path), importPath)
				if imported, err := ParseFile(absPath); err == nil {
					g.File = imported
				}
				// Silently skip unresolvable imports; Group.File stays nil.
			}
			file.Groups = append(file.Groups, g)
			file.Items = append(file.Items, httpfile.FileItem{IsGroup: true, Index: len(file.Groups) - 1})
			continue
		}

		req, err := p.parseRequest(name)
		if err != nil {
			return nil, err
		}
		req.FilePath = p.path
		file.Requests = append(file.Requests, req)
		file.Items = append(file.Items, httpfile.FileItem{IsGroup: false, Index: len(file.Requests) - 1})
	}

	return file, nil
}

// peekImport looks ahead from p.pos, skipping blank lines and # comments, and
// returns the import path if the next substantive line is an @import directive.
func (p *p) peekImport() (path string, ok bool) {
	for i := p.pos; i < len(p.lines); i++ {
		line := strings.TrimSpace(p.lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "@import ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "@import ")), true
		}
		return "", false
	}
	return "", false
}

// consumeImport advances p.pos past the @import line (and any preceding blank/comment lines).
func (p *p) consumeImport() {
	for !p.eof() {
		line := strings.TrimSpace(p.peek())
		p.pos++
		if strings.HasPrefix(line, "@import ") {
			return
		}
	}
}

// parseRequest consumes lines for one request block (stops at the next ### or EOF).
func (p *p) parseRequest(name string) (httpfile.Request, error) {
	req := httpfile.Request{Name: name}

	for !p.eof() {
		if strings.HasPrefix(strings.TrimSpace(p.peek()), "###") {
			break
		}

		line := strings.TrimSpace(p.peek())

		switch {
		case line == "":
			p.pos++

		case strings.HasPrefix(line, "#"): // comment — skip
			p.pos++

		case line == "@pre-request {%":
			p.pos++
			script, err := p.parseScriptBlock()
			if err != nil {
				return req, err
			}
			req.PreScript = script

		case line == "@post-response {%":
			p.pos++
			script, err := p.parseScriptBlock()
			if err != nil {
				return req, err
			}
			req.PostScript = script

		case line == "@example {%":
			p.pos++
			ex, err := p.parseExampleBlock()
			if err != nil {
				return req, err
			}
			req.Example = ex

		case strings.HasPrefix(line, "@jq "):
			p.pos++
			filter := strings.TrimSpace(strings.TrimPrefix(line, "@jq "))
			req.JQFilters = append(req.JQFilters, filter)

		case strings.HasPrefix(line, "@") && !isBlockTag(line):
			p.pos++
			if v, err := parseVarDecl(line); err == nil {
				req.Variables = append(req.Variables, v)
			}

		default:
			// Only try to parse the method line once.
			if req.Method == "" {
				if method, url, ok := parseMethodLine(line); ok {
					p.pos++
					req.Method = method
					req.URL = url
					headers, body, err := p.parseHeadersAndBody()
					if err != nil {
						return req, err
					}
					req.Headers = headers
					req.Body = body
					continue
				}
			}
			p.pos++ // skip unrecognised line
		}
	}

	return req, nil
}

// parseScriptBlock reads lines into the script until the closing %} line.
func (p *p) parseScriptBlock() (string, error) {
	var lines []string
	for !p.eof() {
		line := p.next()
		if strings.TrimSpace(line) == "%}" {
			return trimTrailingBlank(lines), nil
		}
		lines = append(lines, line)
	}
	return trimTrailingBlank(lines), fmt.Errorf("unclosed script block")
}

// parseHeadersAndBody reads headers then an optional body following a blank line.
func (p *p) parseHeadersAndBody() ([]httpfile.Header, string, error) {
	var headers []httpfile.Header

	for !p.eof() {
		raw := p.peek()
		line := strings.TrimSpace(raw)
		if line == "" || isBlockTag(line) || strings.HasPrefix(line, "###") || strings.HasPrefix(line, "@jq ") {
			break
		}
		p.pos++
		if strings.HasPrefix(line, "#") {
			continue // comment — skip
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers = append(headers, httpfile.Header{
			Name:  strings.TrimSpace(name),
			Value: strings.TrimSpace(value),
		})
	}

	var body string
	if !p.eof() && strings.TrimSpace(p.peek()) == "" {
		p.pos++ // consume the blank separator line
		var bodyLines []string
		for !p.eof() {
			raw := p.peek()
			line := strings.TrimSpace(raw)
			if isBlockTag(line) || strings.HasPrefix(line, "###") || strings.HasPrefix(line, "@jq ") {
				break
			}
			p.pos++
			bodyLines = append(bodyLines, raw)
		}
		body = trimTrailingBlank(bodyLines)
	}

	return headers, body, nil
}

// parseExampleBlock reads a saved HTTP response example until the closing %}.
func (p *p) parseExampleBlock() (*httpfile.Example, error) {
	ex := &httpfile.Example{}

	p.skipBlank()
	if p.eof() {
		return ex, nil
	}

	// Status line: "HTTP/1.1 200 OK"
	ex.Status = strings.TrimSpace(p.next())

	// Response headers
	for !p.eof() {
		line := strings.TrimSpace(p.peek())
		if line == "" || line == "%}" {
			break
		}
		p.pos++
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		ex.Headers = append(ex.Headers, httpfile.Header{
			Name:  strings.TrimSpace(name),
			Value: strings.TrimSpace(value),
		})
	}

	// Body (separated from headers by a blank line)
	if !p.eof() && strings.TrimSpace(p.peek()) == "" {
		p.pos++ // consume blank separator
		var bodyLines []string
		for !p.eof() {
			raw := p.peek()
			if strings.TrimSpace(raw) == "%}" {
				p.pos++
				break
			}
			p.pos++
			bodyLines = append(bodyLines, raw)
		}
		ex.Body = trimTrailingBlank(dedentLines(bodyLines))
	} else if !p.eof() && strings.TrimSpace(p.peek()) == "%}" {
		p.pos++ // no body; consume closing %}
	}

	return ex, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isBlockTag reports whether line is the opening tag of a script/example block.
// Using exact matches rather than HasPrefix("@") avoids false positives inside
// JSON bodies that start with @.
func isBlockTag(line string) bool {
	return line == "@pre-request {%" ||
		line == "@post-response {%" ||
		line == "@example {%"
}

func parseVarDecl(line string) (httpfile.Variable, error) {
	line = strings.TrimPrefix(line, "@")
	name, value, ok := strings.Cut(line, "=")
	if !ok {
		return httpfile.Variable{}, fmt.Errorf("invalid variable declaration: %q", line)
	}
	return httpfile.Variable{
		Name:  strings.TrimSpace(name),
		Value: strings.TrimSpace(value),
	}, nil
}

func parseMethodLine(line string) (method, url string, ok bool) {
	m, rest, found := strings.Cut(line, " ")
	if !found {
		return "", "", false
	}
	m = strings.ToUpper(m)
	if !httpMethods[m] {
		return "", "", false
	}
	return m, strings.TrimSpace(rest), true
}

func trimTrailingBlank(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// dedentLines strips the minimum common leading whitespace from all non-empty
// lines. This lets example bodies be indented in the file for readability
// without the indent leaking into the stored/displayed content.
func dedentLines(lines []string) []string {
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if len(line) >= minIndent {
			out[i] = line[minIndent:]
		} else {
			out[i] = line
		}
	}
	return out
}
