package envfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jefferson-julio/h77p/internal/envfile"
)

func TestParse(t *testing.T) {
	src := `# comment
KEY=value
QUOTED="hello world"
SINGLE='hi'
EXPORT_VAR=export_prefix
export EXPORTED=yes
EMPTY=
`
	tmp := t.TempDir()
	f := filepath.Join(tmp, ".env")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := envfile.Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	cases := map[string]string{
		"KEY":          "value",
		"QUOTED":       "hello world",
		"SINGLE":       "hi",
		"EXPORT_VAR":   "export_prefix",
		"EXPORTED":     "yes",
		"EMPTY":        "",
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("key %q: got %q, want %q", k, got[k], want)
		}
	}
	// Comments and blank lines must not produce keys.
	if len(got) != len(cases) {
		t.Errorf("unexpected keys: %v", got)
	}
}

func TestLoad_Priority(t *testing.T) {
	// Layout:
	//   root/           CWD  — defines BASE=root, ONLY_ROOT=1
	//   root/sub/       httpDir — defines BASE=sub, ONLY_SUB=2
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	write := func(dir, content string) {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(root, "BASE=root\nONLY_ROOT=1\n")
	write(sub, "BASE=sub\nONLY_SUB=2\n")

	vars := envfile.Load(sub, root)

	if vars["BASE"] != "sub" {
		t.Errorf("BASE: got %q, want %q", vars["BASE"], "sub")
	}
	if vars["ONLY_ROOT"] != "1" {
		t.Errorf("ONLY_ROOT: got %q, want %q", vars["ONLY_ROOT"], "1")
	}
	if vars["ONLY_SUB"] != "2" {
		t.Errorf("ONLY_SUB: got %q, want %q", vars["ONLY_SUB"], "2")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	// No .env file exists — should return empty map without error.
	vars := envfile.Load(tmp, tmp)
	if len(vars) != 0 {
		t.Errorf("expected empty map, got %v", vars)
	}
}

func TestLoad_OutsideCWD(t *testing.T) {
	// httpDir is outside cwd — only check httpDir's .env.
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, ".env"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	vars := envfile.Load(outside, root)
	if vars["X"] != "1" {
		t.Errorf("X: got %q, want %q", vars["X"], "1")
	}
}
