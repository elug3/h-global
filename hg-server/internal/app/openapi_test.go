package app_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// specOperations reads "METHOD /path" pairs from api/openapi.yaml. It relies
// only on the file's layout (paths at two spaces, methods at four), so it
// needs no YAML library.
func specOperations(t *testing.T) []string {
	f, err := os.Open(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("open spec: %v", err)
	}
	defer f.Close()
	pathRe := regexp.MustCompile(`^  (/\S*):$`)
	methodRe := regexp.MustCompile(`^    (get|post|put|patch|delete):$`)
	var ops []string
	inPaths, cur := false, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "paths:" {
			inPaths = true
			continue
		}
		if inPaths && line != "" && !strings.HasPrefix(line, " ") {
			break
		}
		if m := pathRe.FindStringSubmatch(line); inPaths && m != nil {
			cur = m[1]
		} else if m := methodRe.FindStringSubmatch(line); inPaths && m != nil && cur != "" {
			ops = append(ops, strings.ToUpper(m[1])+" "+cur)
		}
	}
	return ops
}

// TestRoutesMatchOpenAPI keeps api/openapi.yaml honest: every route the
// server registers is documented, and nothing documented is missing.
func TestRoutesMatchOpenAPI(t *testing.T) {
	a, _ := setup(t)
	var routes []string
	for _, r := range a.Routes {
		routes = append(routes, strings.ReplaceAll(r, "...}", "}"))
	}
	spec := specOperations(t)
	for _, r := range routes {
		if !slices.Contains(spec, r) {
			t.Errorf("route %q is not in api/openapi.yaml", r)
		}
	}
	for _, s := range spec {
		if !slices.Contains(routes, s) {
			t.Errorf("api/openapi.yaml documents %q, which the server does not register", s)
		}
	}
}
