package executor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jefferson-julio/h77p/internal/httpfile"
)

func TestInterpolate(t *testing.T) {
	vars := map[string]string{"base": "https://api.example.com", "id": "42"}

	cases := []struct{ in, want string }{
		{"{{base}}/users", "https://api.example.com/users"},
		{"{{base}}/posts/{{id}}", "https://api.example.com/posts/42"},
		{"no vars", "no vars"},
		{"{{missing}}", "{{missing}}"},
		{"{{ id }}", "42"},
	}
	for _, c := range cases {
		got := interpolate(c.in, vars)
		if got != c.want {
			t.Errorf("interpolate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExecuteGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %s, want GET", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	req := &httpfile.Request{
		Method:  "GET",
		URL:     "{{base}}/data",
		Headers: []httpfile.Header{{Name: "Accept", Value: "application/json"}},
	}
	vars := map[string]string{"base": srv.URL}

	result, err := Execute(req, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", result.StatusCode)
	}
	if result.FinalURL != srv.URL+"/data" {
		t.Errorf("FinalURL: got %q", result.FinalURL)
	}
	if result.Body != `{"ok":true}` {
		t.Errorf("body: got %q", result.Body)
	}
}

func TestExecutePOST(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["name"] != "Alice" {
			t.Errorf("body.name: got %q, want Alice", body["name"])
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()

	req := &httpfile.Request{
		Method:  "POST",
		URL:     srv.URL + "/users",
		Headers: []httpfile.Header{{Name: "Content-Type", Value: "application/json"}},
		Body:    `{"name":"Alice"}`,
	}

	result, err := Execute(req, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusCreated {
		t.Errorf("status: got %d, want 201", result.StatusCode)
	}
}
