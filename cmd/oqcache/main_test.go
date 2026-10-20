package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"testing"
)

func TestHitCorruptionFailureAndDependencyChange(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source")
	spec := filepath.Join(dir, "spec.json")
	count := filepath.Join(dir, "count")
	script := filepath.Join(dir, "check.sh")
	os.WriteFile(input, []byte("good"), 0644)
	os.WriteFile(script, []byte(`set -eu
echo run >> "$3"
if grep -q bad "$2"; then exit 9; fi
mkdir "$1"
cp "$2" "$1/artifact"
`), 0644)
	b, _ := json.Marshal(cache.Spec{Paths: []string{input, script}})
	os.WriteFile(spec, b, 0644)
	args := os.Args
	t.Cleanup(func() { os.Args = args })
	os.Args = []string{"oqcache", "run", spec, filepath.Join(dir, "cache"), "--", "bash", script, "{cache}", input, count}
	runs := func() int { b, _ := os.ReadFile(count); return len(b) / 4 }
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if runs() != 1 {
		t.Fatal("cache hit reran command")
	}
	artifacts, _ := filepath.Glob(filepath.Join(dir, "cache", "*", "outputs", "artifact"))
	os.WriteFile(artifacts[0], []byte("corrupt"), 0644)
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if runs() != 2 {
		t.Fatal("corrupt artifact reused")
	}
	os.WriteFile(input, []byte("bad"), 0644)
	if err := run(); err == nil {
		t.Fatal("failed command cached")
	}
	if err := run(); err == nil {
		t.Fatal("failed result reused")
	}
	if runs() != 4 {
		t.Fatal("failure not rerun")
	}
}
