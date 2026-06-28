package executor

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func generateBoundary() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("multipart: generate boundary: %v", err))
	}
	return fmt.Sprintf("----FormBoundary%x", b)
}

// buildMultipartBody replaces [boundary] / [boundary--] tokens with a generated
// MIME boundary string and expands "< filepath" body lines with the file's bytes.
// fileDir is used to resolve relative paths.
// Returns (body bytes, Content-Type value, error).
func buildMultipartBody(rawBody, fileDir string) ([]byte, string, error) {
	boundary := generateBoundary()
	lines := strings.Split(strings.ReplaceAll(rawBody, "\r\n", "\n"), "\n")

	var buf bytes.Buffer
	var partLines []string
	inPart := false

	flushPart := func() error {
		if !inPart || len(partLines) == 0 {
			return nil
		}
		return writeMultipartPart(&buf, partLines, fileDir)
	}

	for _, line := range lines {
		t := strings.TrimSpace(line)

		if t == "[boundary--]" {
			if err := flushPart(); err != nil {
				return nil, "", err
			}
			fmt.Fprintf(&buf, "--%s--\r\n", boundary)
			return buf.Bytes(), "multipart/form-data; boundary=" + boundary, nil
		}

		if t == "[boundary]" {
			if err := flushPart(); err != nil {
				return nil, "", err
			}
			fmt.Fprintf(&buf, "--%s\r\n", boundary)
			partLines = nil
			inPart = true
			continue
		}

		if inPart {
			partLines = append(partLines, line)
		}
	}

	// No explicit [boundary--] — flush last part and close.
	if err := flushPart(); err != nil {
		return nil, "", err
	}
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	return buf.Bytes(), "multipart/form-data; boundary=" + boundary, nil
}

// writeMultipartPart writes a single part (already stripped of its leading boundary
// line) into buf. The part lines follow MIME structure: headers, blank line, body.
// If the body is a single "< filepath" line the file bytes are written directly.
// If no Content-Type header is present and the body is a file, one is auto-detected.
func writeMultipartPart(buf *bytes.Buffer, lines []string, fileDir string) error {
	// Find the blank line separating headers from body.
	splitAt := len(lines)
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			splitAt = i
			break
		}
	}
	headerLines := lines[:splitAt]

	// Body lines (everything after the blank line, trailing empties stripped).
	bodyLines := []string{}
	if splitAt < len(lines) {
		bodyLines = lines[splitAt+1:]
		for len(bodyLines) > 0 && strings.TrimSpace(bodyLines[len(bodyLines)-1]) == "" {
			bodyLines = bodyLines[:len(bodyLines)-1]
		}
	}

	// Check for a file reference in the body.
	var fileData []byte
	var fileName string
	if len(bodyLines) == 1 {
		if trimmed := strings.TrimSpace(bodyLines[0]); strings.HasPrefix(trimmed, "<") {
			relPath := strings.TrimSpace(trimmed[1:])
			absPath := filepath.Join(fileDir, relPath)
			data, err := os.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("read %q: %w", relPath, err)
			}
			fileData = data
			fileName = filepath.Base(absPath)
		}
	}

	// Auto-inject Content-Type for file parts when the user omitted it.
	hasContentType := false
	for _, h := range headerLines {
		if strings.HasPrefix(strings.ToLower(h), "content-type:") {
			hasContentType = true
			break
		}
	}
	if fileData != nil && !hasContentType {
		headerLines = append(headerLines, "Content-Type: "+mimeTypeForFile(fileName))
	}

	// Write headers.
	for _, h := range headerLines {
		fmt.Fprintf(buf, "%s\r\n", strings.TrimRight(h, "\r"))
	}
	buf.WriteString("\r\n")

	// Write body.
	if fileData != nil {
		buf.Write(fileData)
		buf.WriteString("\r\n")
		return nil
	}
	for i, l := range bodyLines {
		if i > 0 {
			buf.WriteString("\r\n")
		}
		buf.WriteString(strings.TrimRight(l, "\r"))
	}
	if len(bodyLines) > 0 {
		buf.WriteString("\r\n")
	}
	return nil
}

// mimeTypeForFile returns a Content-Type for the filename, falling back to
// application/octet-stream.
func mimeTypeForFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".mp3":
		return "audio/mpeg"
	case ".mp4":
		return "video/mp4"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".zip":
		return "application/zip"
	}
	return "application/octet-stream"
}
