package writer

import (
	"fmt"
	"os"
	"strings"

	"github.com/jefferson-julio/h77p/internal/httpfile"
)

// SaveExample writes (or replaces) the @example block for the named request in
// the .http file at path. If requestName is empty the first request is targeted.
func SaveExample(path, requestName string, example *httpfile.Example) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := upsertExample(string(data), requestName, example)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func SetVariable(path, name, value string) error {
	return nil
}

// SaveJQFilters replaces all @jq lines for the named request with filters.
// An empty slice removes all @jq lines.
func SaveJQFilters(path, requestName string, filters []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := upsertJQFilters(string(data), requestName, filters)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func upsertJQFilters(src, requestName string, filters []string) (string, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")

	blockStart, blockEnd, err := findSection(lines, requestName)
	if err != nil {
		return src, err
	}

	// Rebuild block without @jq lines, collapsing consecutive blank lines so we
	// don't leave behind orphaned blank lines where @jq entries used to be.
	var stripped []string
	prevBlank := false
	for _, line := range lines[blockStart:blockEnd] {
		if strings.HasPrefix(strings.TrimSpace(line), "@jq ") {
			continue
		}
		isBlank := strings.TrimSpace(line) == ""
		if isBlank && prevBlank {
			continue
		}
		stripped = append(stripped, line)
		prevBlank = isBlank
	}
	// Remove trailing blank lines from the stripped block.
	for len(stripped) > 1 && strings.TrimSpace(stripped[len(stripped)-1]) == "" {
		stripped = stripped[:len(stripped)-1]
	}

	if len(filters) == 0 {
		result := append(lines[:blockStart:blockStart], stripped...)
		result = append(result, lines[blockEnd:]...)
		return strings.Join(result, "\n"), nil
	}

	// Find insertion point: before @post-response {%, @example {%, or end of block.
	insertAt := len(stripped)
	for i, line := range stripped {
		t := strings.TrimSpace(line)
		if t == "@post-response {%" || t == "@example {%" {
			insertAt = i
			break
		}
	}
	// Retract past any blank lines sitting just before the insertion point.
	for insertAt > 1 && strings.TrimSpace(stripped[insertAt-1]) == "" {
		insertAt--
	}
	// Skip blank lines that immediately follow the insertion point so we don't
	// double-up when we add our own blank separator below.
	afterAt := insertAt
	for afterAt < len(stripped) && strings.TrimSpace(stripped[afterAt]) == "" {
		afterAt++
	}

	var jqLines []string
	for _, f := range filters {
		jqLines = append(jqLines, "@jq "+f)
	}

	newBlock := make([]string, 0, len(stripped)+len(jqLines)+2)
	newBlock = append(newBlock, stripped[:insertAt]...)
	newBlock = append(newBlock, "")
	newBlock = append(newBlock, jqLines...)
	if afterAt < len(stripped) {
		newBlock = append(newBlock, "")
		newBlock = append(newBlock, stripped[afterAt:]...)
	}

	result := append(lines[:blockStart:blockStart], newBlock...)
	result = append(result, lines[blockEnd:]...)
	return strings.Join(result, "\n"), nil
}

var httpMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true, "TRACE": true, "CONNECT": true,
}

func isHTTPMethodLine(line string) bool {
	t := strings.TrimSpace(line)
	m, _, ok := strings.Cut(t, " ")
	return ok && httpMethods[strings.ToUpper(m)]
}

func isHTTPBlockTag(line string) bool {
	t := strings.TrimSpace(line)
	return t == "@pre-request {%" || t == "@post-response {%" || t == "@example {%"
}

// ExtractRequestBlock returns the raw lines for the named request block
// (from ### Name inclusive to the next ### exclusive).
func ExtractRequestBlock(path, requestName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	src := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(src, "\n")
	blockStart, blockEnd, err := findSection(lines, requestName)
	if err != nil {
		return "", err
	}
	return strings.Join(lines[blockStart:blockEnd], "\n"), nil
}

// SaveRequestBlock replaces the named request block in the file with content.
func SaveRequestBlock(path, requestName, content string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(src, "\n")
	blockStart, blockEnd, err := findSection(lines, requestName)
	if err != nil {
		return err
	}
	newLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	result := append(lines[:blockStart:blockStart], newLines...)
	result = append(result, lines[blockEnd:]...)
	return os.WriteFile(path, []byte(strings.Join(result, "\n")), 0o644)
}

// findSection returns the index of the ### requestName line and the exclusive
// end of that block (next ### line or EOF).
func findSection(lines []string, requestName string) (blockStart, blockEnd int, err error) {
	blockStart = -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "###") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(t, "###"))
		if name == requestName {
			blockStart = i
			break
		}
	}
	if blockStart < 0 {
		return 0, 0, fmt.Errorf("request %q not found", requestName)
	}
	blockEnd = len(lines)
	for i := blockStart + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "###") {
			blockEnd = i
			break
		}
	}
	return blockStart, blockEnd, nil
}

// SaveScript writes (or replaces) a script block for the named request.
// kind must be "pre-request" or "post-response".
func SaveScript(path, requestName, kind, script string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := upsertScript(string(data), requestName, kind, script)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func upsertScript(src, requestName, kind, script string) (string, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")

	blockStart, blockEnd, err := findSection(lines, requestName)
	if err != nil {
		return src, err
	}

	tag := "@" + kind + " {%"

	// Look for an existing block.
	exStart, exEnd := -1, -1
	for i := blockStart; i < blockEnd; i++ {
		if strings.TrimSpace(lines[i]) == tag {
			exStart = i
			for j := i + 1; j < blockEnd; j++ {
				if strings.TrimSpace(lines[j]) == "%}" {
					exEnd = j
					break
				}
			}
			break
		}
	}

	newBlock := buildScriptBlock(kind, script)

	var result []string
	if exStart >= 0 && exEnd >= 0 {
		result = append(lines[:exStart:exStart], newBlock...)
		result = append(result, lines[exEnd+1:]...)
		return strings.Join(result, "\n"), nil
	}

	if kind == "pre-request" {
		// Insert before the HTTP method line.
		methodLine := -1
		for i := blockStart + 1; i < blockEnd; i++ {
			if isHTTPMethodLine(lines[i]) {
				methodLine = i
				break
			}
		}
		if methodLine < 0 {
			return src, fmt.Errorf("no HTTP method line found in request %q", requestName)
		}
		result = append(lines[:methodLine:methodLine], newBlock...)
		result = append(result, "")
		result = append(result, lines[methodLine:]...)
	} else {
		// post-response: insert after body, before @example or end of section.
		insertAt := blockEnd
		for i := blockStart + 1; i < blockEnd; i++ {
			if strings.TrimSpace(lines[i]) == "@example {%" {
				insertAt = i
				break
			}
		}
		for insertAt > blockStart+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		result = append(lines[:insertAt:insertAt], "")
		result = append(result, newBlock...)
		result = append(result, lines[insertAt:]...)
	}

	return strings.Join(result, "\n"), nil
}

func buildScriptBlock(kind, script string) []string {
	block := []string{"@" + kind + " {%"}
	for _, line := range strings.Split(script, "\n") {
		if strings.TrimSpace(line) == "" {
			block = append(block, "")
		} else {
			block = append(block, "  "+line)
		}
	}
	block = append(block, "%}")
	return block
}

// SaveRequestLines writes (or replaces) the method line, headers, and body for
// the named request.
func SaveRequestLines(path, requestName, method, url string, headers []httpfile.Header, body string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, err := upsertRequestLines(string(data), requestName, method, url, headers, body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}

func upsertRequestLines(src, requestName, method, url string, headers []httpfile.Header, body string) (string, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")

	blockStart, blockEnd, err := findSection(lines, requestName)
	if err != nil {
		return src, err
	}

	methodLine := -1
	for i := blockStart + 1; i < blockEnd; i++ {
		if isHTTPMethodLine(lines[i]) {
			methodLine = i
			break
		}
	}
	if methodLine < 0 {
		return src, fmt.Errorf("no HTTP method line found in request %q", requestName)
	}

	// Find the end of the request content (headers + body), stopping at block tags,
	// @jq lines, or the next ### separator.
	reqEnd := methodLine + 1
	for reqEnd < blockEnd {
		t := strings.TrimSpace(lines[reqEnd])
		if isHTTPBlockTag(t) || strings.HasPrefix(t, "###") || strings.HasPrefix(t, "@jq ") {
			break
		}
		reqEnd++
	}
	for reqEnd > methodLine+1 && strings.TrimSpace(lines[reqEnd-1]) == "" {
		reqEnd--
	}

	var newLines []string
	newLines = append(newLines, method+" "+url)
	for _, h := range headers {
		newLines = append(newLines, h.Name+": "+h.Value)
	}
	if body != "" {
		newLines = append(newLines, "")
		newLines = append(newLines, strings.Split(strings.TrimRight(body, "\n"), "\n")...)
	}

	result := append(lines[:methodLine:methodLine], newLines...)
	result = append(result, lines[reqEnd:]...)
	return strings.Join(result, "\n"), nil
}

// upsertExample finds the request block in src and replaces or inserts an
// @example {%...%} block. All line-ending handling is normalised to LF.
func upsertExample(src, requestName string, ex *httpfile.Example) (string, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	lines := strings.Split(src, "\n")

	// Find the target request block (### name).
	blockStart := -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "###") {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(t, "###"))
		if name == requestName {
			blockStart = i
			break
		}
	}
	if blockStart < 0 {
		return src, fmt.Errorf("request %q not found", requestName)
	}

	// Find the block end: next ### line or EOF.
	blockEnd := len(lines)
	for i := blockStart + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "###") {
			blockEnd = i
			break
		}
	}

	// Look for an existing @example {%...%} within the block.
	exStart, exEnd := -1, -1
	for i := blockStart; i < blockEnd; i++ {
		if strings.TrimSpace(lines[i]) == "@example {%" {
			exStart = i
			for j := i + 1; j < blockEnd; j++ {
				if strings.TrimSpace(lines[j]) == "%}" {
					exEnd = j
					break
				}
			}
			break
		}
	}

	newBlock := buildExampleBlock(ex)

	var result []string
	if exStart >= 0 && exEnd >= 0 {
		// Replace existing block.
		result = append(lines[:exStart:exStart], newBlock...)
		result = append(result, lines[exEnd+1:]...)
	} else {
		// Insert before blockEnd, after trimming trailing blank lines of the block.
		insertAt := blockEnd
		for insertAt > blockStart+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
			insertAt--
		}
		result = append(lines[:insertAt:insertAt], "")
		result = append(result, newBlock...)
		result = append(result, lines[insertAt:]...)
	}

	return strings.Join(result, "\n"), nil
}

func buildExampleBlock(ex *httpfile.Example) []string {
	lines := []string{"@example {%", "  " + ex.Status}
	for _, h := range ex.Headers {
		lines = append(lines, "  "+h.Name+": "+h.Value)
	}
	if ex.Body != "" {
		lines = append(lines, "")
		for _, bodyLine := range strings.Split(ex.Body, "\n") {
			if strings.TrimSpace(bodyLine) == "" {
				lines = append(lines, "")
			} else {
				lines = append(lines, "  "+bodyLine)
			}
		}
	}
	lines = append(lines, "%}")
	return lines
}
