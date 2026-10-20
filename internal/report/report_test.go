package report

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatesKeepsOrderAndSkipsNoise(t *testing.T) {
	log := "  [circuit-json 2s]\ncircuit health: 0 release errors\nnoise\nDRC: 0 violations / 0 unconnected pads\n  ✗ x\nfabrication archive check: 12 exact files\n"
	got, err := Gates(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"circuit health: 0 release errors", "DRC: 0 violations / 0 unconnected pads", "fabrication archive check: 12 exact files"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
}

func TestVerifyManifestDetectsChanges(t *testing.T) {
	dir := t.TempDir()
	good, bad := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(good, []byte("alpha"), 0o644)
	os.WriteFile(bad, []byte("beta"), 0o644)
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	m := filepath.Join(dir, "manifest.txt")
	os.WriteFile(m, []byte("header\nSHA-256\n"+sum("alpha")+"  a.txt\n"+sum("changed")+"  b.txt\n"+sum("x")+"  missing.txt\n"), 0o644)
	ok, total, problems, err := VerifyManifest(m, dir)
	if err != nil || ok != 1 || total != 3 || len(problems) != 2 {
		t.Fatalf("ok=%d total=%d problems=%v err=%v", ok, total, problems, err)
	}
}

func TestUpdateSectionIsIdempotent(t *testing.T) {
	s := "<!-- oqreport:start -->\nnew evidence\n<!-- oqreport:end -->"
	first, err := UpdateSection("# Report\n", s)
	if err != nil {
		t.Fatal(err)
	}
	second, err := UpdateSection(first, s)
	if err != nil || first != second {
		t.Fatal("section duplicated")
	}
	if _, err = UpdateSection("<!-- oqreport:start -->", s); err == nil {
		t.Fatal("broken markers accepted")
	}
}
