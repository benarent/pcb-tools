package validate

import (
	"context"
	"os"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"strings"
	"testing"
)

func TestParallelIsolationRankAndFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "package.sh")
	// A rendezvous proves both workers run before either finishes. Files are distinct.
	os.WriteFile(script, []byte(`set -eu
mkdir "$3"
printf '%s' "$2" > "$3/input"
touch "$(dirname "$3")/ready-$(basename "$3")"
for i in $(seq 1 100); do
  count=$(ls "$(dirname "$3")"/ready-* | wc -l)
  [ "$count" -ge 2 ] && break
  sleep .01
done
[ "$count" -ge 2 ]
if grep -q bad "$2"; then exit 23; fi
`), 0644)
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("bad"), 0644)
	os.WriteFile(b, []byte("good"), 0644)
	r, err := Run(context.Background(), script, filepath.Join(dir, "out"), []string{a, b}, 2)
	if err != nil || len(r) != 2 || r[0].Passed || !r[1].Passed || r[0].Output == r[1].Output {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = Run(context.Background(), script, filepath.Join(dir, "out"), []string{b}, 1); err == nil {
		t.Fatal("existing output overwritten")
	}
}

func TestSharedCheckReuseAndInvalidation(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "package.sh")
	dep := filepath.Join(dir, "source")
	os.WriteFile(dep, []byte("v1"), 0644)
	os.WriteFile(script, []byte(`set -eu
if [ "$1" = check ]; then
 mkdir "$OQ_CHECK_OUTPUT"
 printf 'source' > "$OQ_CHECK_OUTPUT/result"
 printf 'once\n' >> "$(dirname "$OQ_CHECK_OUTPUT")/calls"
else
 test -n "$OQ_SHARED_CHECK"
 mkdir "$3"
fi
`), 0644)
	spec := filepath.Join(dir, "spec.json")
	cache.WriteJSON(spec, cache.Spec{Paths: []string{dep, script}})
	receipt, err := PrepareShared(context.Background(), script, spec, filepath.Join(dir, "shared"))
	if err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("a"), 0644)
	os.WriteFile(b, []byte("b"), 0644)
	results, err := RunWithShared(context.Background(), script, filepath.Join(dir, "out"), []string{a, b}, 2, receipt)
	if err != nil || !results[0].Passed || !results[1].Passed {
		t.Fatal(results, err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "shared", "calls"))
	if string(calls) != "once\n" {
		t.Fatal("source checks repeated", string(calls))
	}
	os.WriteFile(dep, []byte("v2"), 0644)
	if _, err = VerifyShared(receipt); err == nil {
		t.Fatal("changed dependency reused")
	}
	os.WriteFile(dep, []byte("v1"), 0644)
	os.WriteFile(filepath.Join(dir, "shared", "artifacts", "result"), []byte("tamper"), 0644)
	if _, err = VerifyShared(receipt); err == nil {
		t.Fatal("changed artifact reused")
	}
}

func TestAdoptCacheChecksCommandAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input")
	os.WriteFile(input, []byte("source"), 0644)
	specPath := filepath.Join(dir, "deps.json")
	spec := cache.Spec{Paths: []string{input}}
	cache.WriteJSON(specPath, spec)
	artifacts := filepath.Join(dir, "cache", "outputs")
	os.MkdirAll(artifacts, 0755)
	os.WriteFile(filepath.Join(artifacts, "result"), []byte("ok"), 0644)
	spec.Values = map[string]string{"command": strings.Join([]string{"env", "OQ_CHECK_OUTPUT={cache}", "bash", "package.sh", "check"}, "\x00")}
	key, _ := cache.Key(spec)
	outputKey, _ := cache.Key(cache.Spec{Paths: []string{artifacts}})
	cache.WriteJSON(filepath.Join(dir, "cache", "receipt.json"), map[string]string{"Key": key, "Outputs": outputKey})
	receipt, e := AdoptCached("package.sh", specPath, artifacts, filepath.Join(dir, "batch"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = VerifyShared(receipt); e != nil {
		t.Fatal(e)
	}
	if _, e = AdoptCached("other.sh", specPath, artifacts, filepath.Join(dir, "bad")); e == nil {
		t.Fatal("wrong command reused")
	}
	os.WriteFile(filepath.Join(artifacts, "result"), []byte("tamper"), 0644)
	if _, e = AdoptCached("package.sh", specPath, artifacts, filepath.Join(dir, "bad2")); e == nil {
		t.Fatal("tampered cache reused")
	}
}
