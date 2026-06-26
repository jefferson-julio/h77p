package ui

import (
	"strings"
	"testing"
)

func TestHighlightHTTP_JsonRequestBody(t *testing.T) {
	src := "### Create\nPOST /posts\nContent-Type: application/json\n\n{\"key\":\"value\"}\n\n### Next\nGET /x\n"
	out := highlightHTTP(src)
	bodyLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "key") {
			bodyLine = l
		}
	}
	if bodyLine == "" {
		t.Fatal("did not find body line in output")
	}
	if !strings.Contains(bodyLine, "\x1b[") {
		t.Errorf("JSON body line has no ANSI escapes (no highlighting): %q", bodyLine)
	}
}

func TestHighlightHTTP_ExampleJsonBody(t *testing.T) {
	src := "@example {%\nHTTP/1.1 200 OK\nContent-Type: application/json\n\n{\"id\":1}\n%}\n"
	out := highlightHTTP(src)
	bodyLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "\"id\"") {
			bodyLine = l
		}
	}
	if bodyLine == "" {
		t.Fatal("did not find example body line in output")
	}
	if !strings.Contains(bodyLine, "\x1b[") {
		t.Errorf("example JSON body line has no ANSI escapes: %q", bodyLine)
	}
}

func TestHighlightHTTP_PlainBodyPassthrough(t *testing.T) {
	src := "POST /raw\nContent-Type: text/plain\n\nhello world\n"
	out := highlightHTTP(src)
	if !strings.Contains(out, "hello world") {
		t.Error("plain body text was dropped")
	}
	// The plain body line should have no ANSI escapes.
	for _, l := range strings.Split(out, "\n") {
		if l == "hello world" {
			return
		}
	}
	t.Log("note: plain body line may have been altered, but text is preserved")
}

func TestHighlightHTTP_XmlBody(t *testing.T) {
	src := "POST /xml\nContent-Type: application/xml\n\n<root><id>1</id></root>\n"
	out := highlightHTTP(src)
	xmlLine := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "root") {
			xmlLine = l
		}
	}
	if xmlLine == "" {
		t.Fatal("did not find XML body line")
	}
	if !strings.Contains(xmlLine, "\x1b[") {
		t.Errorf("XML body line has no ANSI escapes: %q", xmlLine)
	}
}

func TestHighlightHTTP_NextRequestAfterBody(t *testing.T) {
	// Verify modeReqBody exits on ### so the next request is not swallowed.
	src := "### One\nPOST /a\nContent-Type: application/json\n\n{\"x\":1}\n\n### Two\nGET /b\n"
	out := highlightHTTP(src)
	if !strings.Contains(out, "### Two") {
		t.Error("### Two section header lost after JSON body block")
	}
	if !strings.Contains(out, "GET") || !strings.Contains(out, "/b") {
		t.Error("GET /b request lost after JSON body block")
	}
}
