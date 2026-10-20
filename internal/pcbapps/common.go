package pcbapps

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"sort"
	"strings"
)

func ReadJSON(path string, v any) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return fmt.Errorf("%s: %w", path, e)
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("%s: trailing JSON", path)
	}
	return nil
}
func Hash(path string) (string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}
func Keys[T any](m map[string]T) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func Write(path string, v any) error {
	if path == "-" {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		return e.Encode(v)
	}
	return cache.WriteJSON(path, v)
}
func SaveReport(path string, v any, issues []string) error {
	if e := Write(path, v); e != nil {
		return e
	}
	if len(issues) > 0 {
		return fmt.Errorf("%d findings; see %s", len(issues), path)
	}
	return nil
}
func SafeRelative(p string) bool {
	return p != "" && p != "." && !filepath.IsAbs(p) && filepath.Clean(p) == p && p != ".." && !strings.HasPrefix(p, ".."+string(filepath.Separator))
}
func Main(name string) {
	var e error
	switch name {
	case "oqroundtrip":
		e = Roundtrip(os.Args[1:])
	case "oqdiff":
		e = Diff(os.Args[1:])
	case "oqfootprint":
		e = Footprint(os.Args[1:])
	case "oqmechanical":
		e = Mechanical(os.Args[1:])
	case "oqrelease":
		e = Release(os.Args[1:])
	case "oqbench":
		e = Bench(os.Args[1:])
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, name+":", e)
		os.Exit(1)
	}
}
