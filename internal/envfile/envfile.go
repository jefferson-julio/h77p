package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Parse reads a .env file and returns its key-value pairs.
// Blank lines and lines beginning with # are ignored. The optional
// "export " prefix is stripped. Values may be wrapped in " or ' quotes
// (outer pair only — inner content is taken verbatim).
func Parse(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			q := val[0]
			if (q == '"' || q == '\'') && val[len(val)-1] == q {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" {
			vars[key] = val
		}
	}
	return vars, sc.Err()
}

// Load discovers .env files from httpDir upward to cwd (inclusive) and merges
// them. Directories closer to httpDir override parent directories. Files that
// do not exist or cannot be read are silently skipped.
func Load(httpDir, cwd string) map[string]string {
	merged := make(map[string]string)
	for _, dir := range walkDirs(httpDir, cwd) {
		vars, err := Parse(filepath.Join(dir, ".env"))
		if err != nil {
			continue
		}
		for k, v := range vars {
			merged[k] = v
		}
	}
	return merged
}

// walkDirs returns the directory chain from cwd down to httpDir, outermost
// first, so later iterations (closer to the file) win. If httpDir lies outside
// cwd, only httpDir itself is returned.
func walkDirs(httpDir, cwd string) []string {
	httpDir = filepath.Clean(httpDir)
	cwd = filepath.Clean(cwd)

	rel, err := filepath.Rel(cwd, httpDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return []string{httpDir}
	}

	// Walk from httpDir up to cwd, collecting each directory.
	var chain []string
	cur := httpDir
	for {
		chain = append(chain, cur)
		if cur == cwd {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break // filesystem root — shouldn't happen given the Rel check
		}
		cur = parent
	}
	// Reverse so outermost (cwd) comes first; inner dirs will overwrite.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
