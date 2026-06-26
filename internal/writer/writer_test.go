package writer

import (
	"strings"
	"testing"

	"github.com/jefferson-julio/h77p/internal/httpfile"
)

var sampleEx = &httpfile.Example{
	Status: "HTTP/1.1 200 OK",
	Headers: []httpfile.Header{
		{Name: "Content-Type", Value: "application/json"},
	},
	Body: `{"id":1}`,
}

const sampleSrc = `@base = https://example.com

### Get Users
GET {{base}}/users
Accept: application/json

### Create User
POST {{base}}/users
Content-Type: application/json

{"name":"Alice"}

### Delete User
DELETE {{base}}/users/1
`

func TestInsertNewExample(t *testing.T) {
	out, err := upsertExample(sampleSrc, "Get Users", sampleEx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "@example {%") {
		t.Error("@example block not inserted")
	}
	if !strings.Contains(out, "HTTP/1.1 200 OK") {
		t.Error("example status missing")
	}
	if !strings.Contains(out, `{"id":1}`) {
		t.Error("example body missing")
	}
	// The next request should still be present.
	if !strings.Contains(out, "### Create User") {
		t.Error("subsequent request lost after insert")
	}
}

func TestReplaceExistingExample(t *testing.T) {
	src := `### Get Users
GET /users

@example {%
HTTP/1.1 404 Not Found
%}

### Other
GET /other
`
	updated, err := upsertExample(src, "Get Users", sampleEx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(updated, "404") {
		t.Error("old example not replaced")
	}
	if !strings.Contains(updated, "HTTP/1.1 200 OK") {
		t.Error("new example status missing")
	}
	if !strings.Contains(updated, "### Other") {
		t.Error("subsequent request lost after replace")
	}
}

func TestInsertInLastBlock(t *testing.T) {
	src := `### Only
GET /only
`
	out, err := upsertExample(src, "Only", sampleEx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "@example {%") {
		t.Error("@example block not inserted in last block")
	}
}

func TestUnknownRequestName(t *testing.T) {
	_, err := upsertExample(sampleSrc, "Missing", sampleEx)
	if err == nil {
		t.Fatal("expected error for missing request name")
	}
}

func TestExampleWithNoBody(t *testing.T) {
	ex := &httpfile.Example{Status: "HTTP/1.1 204 No Content"}
	out, err := upsertExample(sampleSrc, "Delete User", ex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "204 No Content") {
		t.Error("example status missing")
	}
}
