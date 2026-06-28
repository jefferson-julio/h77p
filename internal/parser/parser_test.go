package parser_test

import (
	"strings"
	"testing"

	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/parser"
)

// -----------------------------------------------------------------------------
// Fixtures
// -----------------------------------------------------------------------------

const minimalSrc = `@baseUrl = https://api.example.com

### Get Users
GET {{baseUrl}}/users
Accept: application/json

### Create User

@pre-request {%
  request.headers["X-Id"] = "test";
%}

POST {{baseUrl}}/users
Content-Type: application/json

{"name": "Alice"}

@post-response {%
  assert(response.status === 201);
  set("userId", response.json().id);
%}

@example {%
HTTP/1.1 201 Created
Content-Type: application/json

{"id": "abc", "name": "Alice"}
%}
`

// -----------------------------------------------------------------------------
// File-level declarations
// -----------------------------------------------------------------------------

func TestParseVariables(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Variables) != 1 {
		t.Fatalf("variables: got %d, want 1", len(f.Variables))
	}
	v := f.Variables[0]
	if v.Name != "baseUrl" {
		t.Errorf("name: got %q, want %q", v.Name, "baseUrl")
	}
	if v.Value != "https://api.example.com" {
		t.Errorf("value: got %q, want %q", v.Value, "https://api.example.com")
	}
}

func TestImportAtFileLevelIgnored(t *testing.T) {
	// A top-level @import (without a preceding ### Name) is silently ignored.
	src := "@import ./auth.http\n\n### Ping\nGET /ping\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Groups) != 0 {
		t.Errorf("expected no groups for top-level @import, got %d", len(f.Groups))
	}
	if len(f.Requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(f.Requests))
	}
}

func TestGroupImport(t *testing.T) {
	// ### Name + @import creates a Group. Since path="" the file load fails silently;
	// the group is still created with File==nil.
	src := "### Auth Group\n@import ./auth.http\n\n### Ping\nGET /ping\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(f.Groups))
	}
	g := f.Groups[0]
	if g.Name != "Auth Group" {
		t.Errorf("group name: got %q", g.Name)
	}
	if g.Source != "./auth.http" {
		t.Errorf("group source: got %q", g.Source)
	}
	// File is nil because path is "" and the import cannot be resolved.
	if g.File != nil {
		t.Errorf("expected nil File for unresolvable import")
	}
	if len(f.Requests) != 1 || f.Requests[0].Name != "Ping" {
		t.Errorf("expected 1 top-level request 'Ping', got %v", f.Requests)
	}
}

func TestNestedGroupImport(t *testing.T) {
	// Verifies that groups inside imported files are themselves parsed as groups.
	// sample.http → group1.sample.http (has a nested ### Comments @import group2)
	f, err := parser.ParseFile("../../testdata/sample.http")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	var g1 *httpfile.Group
	for i := range f.Groups {
		if f.Groups[i].Name == "Group 1" {
			g1 = &f.Groups[i]
			break
		}
	}
	if g1 == nil {
		t.Fatalf("group %q not found in top-level groups %v", "Group 1", func() []string {
			var names []string
			for _, g := range f.Groups { names = append(names, g.Name) }
			return names
		}())
	}
	if g1.File == nil {
		t.Fatal("group1 file not loaded")
	}
	if len(g1.File.Groups) != 1 {
		t.Fatalf("expected 1 nested group inside group1, got %d", len(g1.File.Groups))
	}
	g2 := g1.File.Groups[0]
	if g2.Name != "Comments" {
		t.Errorf("nested group name: got %q, want %q", g2.Name, "Comments")
	}
	if g2.File == nil {
		t.Fatal("group2 file not loaded")
	}
	if len(g2.File.Requests) == 0 {
		t.Error("expected requests in group2 file")
	}
}

// -----------------------------------------------------------------------------
// Request count and names
// -----------------------------------------------------------------------------

func TestRequestCount(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 2 {
		t.Fatalf("requests: got %d, want 2", len(f.Requests))
	}
}

func TestAnonymousRequest(t *testing.T) {
	src := "###\nGET /health\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(f.Requests))
	}
	if f.Requests[0].Name != "" {
		t.Errorf("expected empty name, got %q", f.Requests[0].Name)
	}
}

// -----------------------------------------------------------------------------
// Method, URL, headers, body
// -----------------------------------------------------------------------------

func TestGetRequest(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := f.Requests[0]
	if r.Name != "Get Users" {
		t.Errorf("name: got %q, want %q", r.Name, "Get Users")
	}
	if r.Method != "GET" {
		t.Errorf("method: got %q, want GET", r.Method)
	}
	if r.URL != "{{baseUrl}}/users" {
		t.Errorf("url: got %q", r.URL)
	}
	if len(r.Headers) != 1 {
		t.Fatalf("headers: got %d, want 1", len(r.Headers))
	}
	if r.Headers[0].Name != "Accept" || r.Headers[0].Value != "application/json" {
		t.Errorf("header[0]: %q: %q", r.Headers[0].Name, r.Headers[0].Value)
	}
	if r.Body != "" {
		t.Errorf("body: expected empty, got %q", r.Body)
	}
}

func TestPostRequestBody(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := f.Requests[1]
	want := `{"name": "Alice"}`
	if r.Body != want {
		t.Errorf("body: got %q, want %q", r.Body, want)
	}
}

func TestMultilineBody(t *testing.T) {
	src := "### Create\nPOST /items\nContent-Type: application/json\n\n{\n  \"name\": \"x\"\n}\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "{\n  \"name\": \"x\"\n}"
	if f.Requests[0].Body != want {
		t.Errorf("body: got %q, want %q", f.Requests[0].Body, want)
	}
}

func TestNoBodyWhenNoBlankLine(t *testing.T) {
	src := "### Ping\nGET /ping\nAccept: */*\n### Done\nGET /done\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Requests[0].Body != "" {
		t.Errorf("body: expected empty, got %q", f.Requests[0].Body)
	}
	if len(f.Requests[0].Headers) != 1 {
		t.Errorf("headers: got %d, want 1", len(f.Requests[0].Headers))
	}
}

func TestHeaderColonInValue(t *testing.T) {
	src := "### T\nGET /t\nAuthorization: Bearer tok:en\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h := f.Requests[0].Headers[0]
	if h.Name != "Authorization" || h.Value != "Bearer tok:en" {
		t.Errorf("got %q: %q", h.Name, h.Value)
	}
}

// -----------------------------------------------------------------------------
// Script blocks
// -----------------------------------------------------------------------------

func TestPreScript(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := f.Requests[1]
	if r.PreScript == "" {
		t.Fatal("expected pre-request script")
	}
	if !strings.Contains(r.PreScript, `request.headers["X-Id"]`) {
		t.Errorf("pre-script content unexpected: %q", r.PreScript)
	}
}

func TestPostScript(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := f.Requests[1]
	if r.PostScript == "" {
		t.Fatal("expected post-response script")
	}
	if !strings.Contains(r.PostScript, "response.status === 201") {
		t.Errorf("post-script content unexpected: %q", r.PostScript)
	}
}

func TestMultilineScript(t *testing.T) {
	src := "### T\nGET /t\n\n@post-response {%\n  test(\"A\", () => {\n    assert(true);\n  });\n%}\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := f.Requests[0].PostScript
	if !strings.Contains(s, "test(\"A\"") {
		t.Errorf("script: %q", s)
	}
	if !strings.Contains(s, "assert(true)") {
		t.Errorf("script: %q", s)
	}
}

// -----------------------------------------------------------------------------
// Example block
// -----------------------------------------------------------------------------

func TestExampleBlock(t *testing.T) {
	f, err := parser.ParseString(minimalSrc, "test.http")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := f.Requests[1]
	if r.Example == nil {
		t.Fatal("expected example block")
	}
	ex := r.Example
	if ex.Status != "HTTP/1.1 201 Created" {
		t.Errorf("status: got %q", ex.Status)
	}
	if len(ex.Headers) != 1 {
		t.Fatalf("example headers: got %d, want 1", len(ex.Headers))
	}
	if ex.Headers[0].Name != "Content-Type" {
		t.Errorf("example header name: got %q", ex.Headers[0].Name)
	}
	if ex.Body == "" {
		t.Error("expected example body")
	}
	if !strings.Contains(ex.Body, `"name": "Alice"`) {
		t.Errorf("example body: %q", ex.Body)
	}
}

func TestExampleNoBody(t *testing.T) {
	src := "### Delete\nDELETE /x\n\n@example {%\nHTTP/1.1 204 No Content\n%}\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ex := f.Requests[0].Example
	if ex == nil {
		t.Fatal("expected example")
	}
	if ex.Status != "HTTP/1.1 204 No Content" {
		t.Errorf("status: %q", ex.Status)
	}
	if ex.Body != "" {
		t.Errorf("body: expected empty, got %q", ex.Body)
	}
}

// -----------------------------------------------------------------------------
// CRLF and whitespace robustness
// -----------------------------------------------------------------------------

func TestCRLF(t *testing.T) {
	src := "@base = http://x.com\r\n\r\n### T\r\nGET /t\r\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Variables) != 1 || f.Variables[0].Name != "base" {
		t.Errorf("variables: %v", f.Variables)
	}
	if len(f.Requests) != 1 || f.Requests[0].Method != "GET" {
		t.Errorf("requests: %v", f.Requests)
	}
}

func TestTrailingBlankLinesInBody(t *testing.T) {
	src := "### T\nPOST /t\n\n{\"a\":1}\n\n\n### Next\nGET /n\n"
	f, err := parser.ParseString(src, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Requests[0].Body != `{"a":1}` {
		t.Errorf("body: got %q", f.Requests[0].Body)
	}
}

// -----------------------------------------------------------------------------
// Golden test against testdata/sample.http
// -----------------------------------------------------------------------------

func TestSampleFile(t *testing.T) {
	f, err := parser.ParseFile("../../testdata/sample.http")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(f.Variables) != 1 {
		t.Errorf("variables: got %d, want 1", len(f.Variables))
	}
	if f.Variables[0].Name != "host" {
		t.Errorf("variable name: got %q", f.Variables[0].Name)
	}

	if len(f.Requests) < 3 {
		t.Fatalf("requests: got %d, want at least 3", len(f.Requests))
	}

	t.Run("GetAllPosts", func(t *testing.T) {
		r := f.Requests[0]
		if r.Name != "Get All Posts" {
			t.Errorf("name: %q", r.Name)
		}
		if r.Method != "GET" {
			t.Errorf("method: %q", r.Method)
		}
		if r.URL != "/posts" {
			t.Errorf("url: %q", r.URL)
		}
		if len(r.Headers) != 1 || r.Headers[0].Name != "Accept" {
			t.Errorf("headers: %v", r.Headers)
		}
		if r.Body != "" {
			t.Errorf("body: expected empty, got %q", r.Body)
		}
		if r.PostScript == "" {
			t.Error("expected post-response script")
		}
		if r.Example == nil {
			t.Fatal("expected example block")
		}
		if r.Example.Status != "HTTP/1.1 200 OK" {
			t.Errorf("example status: %q", r.Example.Status)
		}
	})

	t.Run("CreatePost", func(t *testing.T) {
		r := f.Requests[1]
		if r.Name != "Create Post" {
			t.Errorf("name: %q", r.Name)
		}
		if r.Method != "POST" {
			t.Errorf("method: %q", r.Method)
		}
		if r.PreScript == "" {
			t.Error("expected pre-request script")
		}
		if r.Body == "" {
			t.Error("expected body")
		}
		if r.PostScript == "" {
			t.Error("expected post-response script")
		}
	})

	t.Run("GetCreatedPost", func(t *testing.T) {
		r := f.Requests[2]
		if r.Name != "Get Created Post" {
			t.Errorf("name: %q", r.Name)
		}
		if r.URL != "/posts/{{createdId}}" {
			t.Errorf("url: %q", r.URL)
		}
	})
}
