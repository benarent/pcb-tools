package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDependenciesInvalidate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "input")
	os.WriteFile(p, []byte("old"), 0644)
	s := Spec{Paths: []string{dir}, Values: map[string]string{"tool": "v1"}}
	a, err := Key(s)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Key(s)
	if a != b {
		t.Fatal("unstable key")
	}
	os.WriteFile(p, []byte("new"), 0644)
	b, _ = Key(s)
	if a == b {
		t.Fatal("content change ignored")
	}
	a = b
	s.Values["tool"] = "v2"
	b, _ = Key(s)
	if a == b {
		t.Fatal("tool change ignored")
	}
	a = b
	os.WriteFile(filepath.Join(dir, "added"), []byte("x"), 0644)
	b, _ = Key(s)
	if a == b {
		t.Fatal("new dependency ignored")
	}
	if _, err = Key(Spec{Paths: []string{filepath.Join(dir, "missing")}}); err == nil {
		t.Fatal("missing input accepted")
	}
}
func TestSymlinkContents(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	os.WriteFile(target, []byte("one"), 0644)
	os.Symlink(target, link)
	a, _ := Key(Spec{Paths: []string{link}})
	os.WriteFile(target, []byte("two"), 0644)
	b, _ := Key(Spec{Paths: []string{link}})
	if a == b {
		t.Fatal("symlink content not hashed")
	}
}
