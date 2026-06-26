package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jefferson-julio/h77p/internal/httpfile"
	"github.com/jefferson-julio/h77p/internal/parser"
)

// newTestServer returns a test server that echoes requests as JSON.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/created":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"id": "99"})
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"path": r.URL.Path})
		}
	}))
}

func TestRunByName(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	file := &httpfile.File{
		Variables: []httpfile.Variable{{Name: "base", Value: srv.URL}},
		Requests: []httpfile.Request{
			{Name: "First", Method: "GET", URL: "{{base}}/first"},
			{Name: "Second", Method: "GET", URL: "{{base}}/second"},
		},
	}

	result, err := Run(file, "Second", make(map[string]string))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result.Err: %v", result.Err)
	}
	if result.HTTP.FinalURL != srv.URL+"/second" {
		t.Errorf("FinalURL: got %q", result.HTTP.FinalURL)
	}
}

func TestRunNotFound(t *testing.T) {
	file := &httpfile.File{}
	_, err := Run(file, "Missing", make(map[string]string))
	if err == nil {
		t.Fatal("expected error for missing request name")
	}
}

func TestRunFirstByDefault(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	file := &httpfile.File{
		Requests: []httpfile.Request{
			{Name: "Only", Method: "GET", URL: srv.URL + "/only"},
		},
	}

	result, err := Run(file, "", make(map[string]string))
	if err != nil || result.Err != nil {
		t.Fatalf("unexpected error: %v / %v", err, result.Err)
	}
	if result.HTTP.StatusCode != http.StatusOK {
		t.Errorf("status: got %d", result.HTTP.StatusCode)
	}
}

func TestPreScript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "injected" {
			t.Errorf("X-Test header not injected, got %q", r.Header.Get("X-Test"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	file := &httpfile.File{
		Requests: []httpfile.Request{{
			Name:      "Pre",
			Method:    "GET",
			URL:       srv.URL + "/pre",
			PreScript: `request.headers["X-Test"] = "injected";`,
		}},
	}

	result, err := Run(file, "", make(map[string]string))
	if err != nil || result.Err != nil {
		t.Fatalf("unexpected error: %v / %v", err, result.Err)
	}
}

func TestPostScriptPassFail(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	file := &httpfile.File{
		Requests: []httpfile.Request{{
			Name:   "Tests",
			Method: "GET",
			URL:    srv.URL + "/x",
			PostScript: `
				test("status is 200", function() {
					assert(response.status === 200, "expected 200");
				});
				test("will fail", function() {
					assert(false, "intentional failure");
				});
			`,
		}},
	}

	result, err := Run(file, "", make(map[string]string))
	if err != nil || result.Err != nil {
		t.Fatalf("unexpected error: %v / %v", err, result.Err)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("tests: got %d, want 2", len(result.Tests))
	}
	if !result.Tests[0].Passed {
		t.Errorf("test[0] should pass")
	}
	if result.Tests[1].Passed {
		t.Errorf("test[1] should fail")
	}
	if result.Tests[1].Error != "intentional failure" {
		t.Errorf("test[1] error: got %q", result.Tests[1].Error)
	}
	if result.Passed {
		t.Error("result.Passed should be false")
	}
}

func TestSetPropagatesVars(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Two requests chained: first captures id via set(), second uses {{capturedId}}.
	src := `
@base = ` + srv.URL + `

### Create
POST {{base}}/created

@post-response {%
  test("status 201", function() { assert(response.status === 201); });
  set("capturedId", String(response.json().id));
%}

### Get
GET {{base}}/items/{{capturedId}}
`
	file, err := parser.ParseString(src, "test.http")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	vars := make(map[string]string)
	results, err := RunAll(file, vars)
	if err != nil {
		t.Fatalf("RunAll error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results: got %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Fatalf("result.Err: %v", r.Err)
		}
	}
	if vars["capturedId"] != "99" {
		t.Errorf("capturedId: got %q, want 99", vars["capturedId"])
	}
	if results[1].HTTP.FinalURL != srv.URL+"/items/99" {
		t.Errorf("second FinalURL: got %q", results[1].HTTP.FinalURL)
	}
}
