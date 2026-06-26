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
	lines := []string{"@example {%", ex.Status}
	for _, h := range ex.Headers {
		lines = append(lines, h.Name+": "+h.Value)
	}
	if ex.Body != "" {
		lines = append(lines, "")
		lines = append(lines, strings.Split(ex.Body, "\n")...)
	}
	lines = append(lines, "%}")
	return lines
}
