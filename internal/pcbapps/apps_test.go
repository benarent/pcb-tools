package pcbapps

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/pcb"
	"strings"
	"testing"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(s), 0644); e != nil {
		t.Fatal(e)
	}
}
func evidence(t *testing.T, dir string) []Evidence {
	put(t, filepath.Join(dir, "drawing.txt"), "reviewed manufacturer drawing fixture")
	h, e := Hash(filepath.Join(dir, "drawing.txt"))
	if e != nil {
		t.Fatal(e)
	}
	return []Evidence{{Path: "drawing.txt", SHA256: h, Revision: "fixture-1", Basis: "test fixture only"}}
}
func fixtureBoard() pcb.Board {
	return pcb.Board{Format: "kicad", Components: map[string]pcb.Component{"J1": {Ref: "J1", Layer: "F.Cu", At: pcb.Point{X: 10, Y: 10}, Pads: []pcb.Pad{{Number: "1", Net: "GND", Kind: "thru_hole", Shape: "oval", At: pcb.Point{X: 10, Y: 10}, Size: pcb.Point{X: 2, Y: 1}, Drill: pcb.Point{X: .8, Y: .4}, Layers: []string{"*.Cu"}}}}}, Layers: []string{"F.Cu", "B.Cu"}, Copper: []pcb.Copper{{Kind: "segment", Net: "GND", Layers: []string{"F.Cu"}, Start: pcb.Point{X: 10, Y: 10}, End: pcb.Point{X: 20, Y: 10}, Width: .2}}}
}
func TestCopperCoverageSplitsAndFaults(t *testing.T) {
	a := fixtureBoard().Copper[0]
	left, right := a, a
	left.End.X = 15
	right.Start.X = 15
	if got := missingCopper([]pcb.Copper{a}, []pcb.Copper{left, right}, .002, true); len(got) != 0 {
		t.Fatal(got)
	}
	right.Start.X = 15.1
	if len(missingCopper([]pcb.Copper{a}, []pcb.Copper{left, right}, .002, true)) == 0 {
		t.Fatal("gap accepted")
	}
	right = a
	right.Net = "VCC"
	if len(missingCopper([]pcb.Copper{a}, []pcb.Copper{right}, .002, true)) == 0 {
		t.Fatal("wrong net accepted")
	}
}
func TestDiffPinsCopperAndIdentity(t *testing.T) {
	a, b := fixtureBoard(), fixtureBoard()
	if r := DesignDiff(a, b); len(r.Changes) != 0 {
		t.Fatal(r)
	}
	c := b.Components["J1"]
	c.Pads[0].Net = "VCC"
	b.Components["J1"] = c
	b.Copper = nil
	r := DesignDiff(a, b)
	if len(r.Changes) < 2 || r.RemovedCopper != 1 {
		t.Fatal(r)
	}
	svg := Overlay(a, b)
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "J1") {
		t.Fatal("missing overlay")
	}
}
func TestFootprintSlotsPinoutAndEvidence(t *testing.T) {
	dir := t.TempDir()
	gnd := "GND"
	rule := FootprintContract{Schema: 1, Status: "reviewed", Tolerance: .001, Footprints: []FootprintRule{{Ref: "J1", PadCount: 1, Evidence: evidence(t, dir), Pads: []PadRule{{Number: "1", Count: 1, Net: &gnd, Drill: &pcb.Point{X: .8, Y: .4}}}}}}
	b := fixtureBoard()
	r, e := CheckFootprints(rule, b, dir)
	if e != nil || !r.Passed {
		t.Fatalf("%+v %v", r, e)
	}
	c := b.Components["J1"]
	c.Pads[0].Drill.Y = .8
	b.Components["J1"] = c
	r, e = CheckFootprints(rule, b, dir)
	if e != nil || r.Passed {
		t.Fatalf("slot fault accepted: %+v %v", r, e)
	}
	put(t, filepath.Join(dir, "drawing.txt"), "changed")
	if _, e = CheckFootprints(rule, b, dir); e == nil {
		t.Fatal("changed evidence accepted")
	}
}
func TestMechanicalRotatedEnvelopeAndUnknownHeight(t *testing.T) {
	dir := t.TempDir()
	c := MechanicalContract{Schema: 1, CoordinateSystem: "kicad-mm-y-down", Envelopes: []Envelope{{ID: "access", Center: pcb.Point{X: 10, Y: 10}, Size: pcb.Point{X: 1, Y: 8}, Rotation: 45, Side: "top", Evidence: evidence(t, dir)}}}
	b := fixtureBoard()
	r, e := CheckMechanical(c, b, dir, false)
	if e != nil || r.Passed {
		t.Fatalf("collision accepted: %+v %v", r, e)
	}
	c.Envelopes[0].ZMin = 3
	r, e = CheckMechanical(c, b, dir, false)
	if e != nil || !strings.Contains(r.Issues[0], "unknown body height") {
		t.Fatalf("%+v %v", r, e)
	}
	c.Bodies = []Body{{Ref: "J1", Size: pcb.Point{X: 2, Y: 1}, Height: 2}}
	r, e = CheckMechanical(c, b, dir, false)
	if e != nil || !r.Passed {
		t.Fatalf("clear height rejected: %+v %v", r, e)
	}
	c.Envelopes[0].Center.X = 20
	r, e = CheckMechanical(c, b, dir, false)
	if e != nil || !r.Passed {
		t.Fatal(r, e)
	}
}
func TestBenchSpectralCalibrationAndEvidence(t *testing.T) {
	dir := t.TempDir()
	var data strings.Builder
	data.WriteString("time_s,signal\n")
	for i := 0; i < 1024; i++ {
		fmt.Fprintf(&data, "%.12f,%.12f\n", float64(i)/1024, 2*math.Sin(2*math.Pi*64*float64(i)/1024))
	}
	put(t, filepath.Join(dir, "capture.csv"), data.String())
	h, _ := Hash(filepath.Join(dir, "capture.csv"))
	limit := 1.5
	p := BenchPlan{Schema: 1, Title: "fixture", Checks: []BenchCheck{{ID: "noise", Requirement: "test", Procedure: "test", Unit: "V", Column: "signal", Statistic: "band_rms", TimeColumn: "time_s", MinSamples: 1024, SampleRate: 1024, LowHz: 60, HighHz: 68, Max: &limit}}}
	run := BenchRun{Schema: 1, Serial: "test-only", PCB: strings.Repeat("a", 64), Firmware: strings.Repeat("b", 64), Measured: "2026-01-01T00:00:00Z", Instruments: []string{"synthetic fixture"}, Captures: []Capture{{Check: "noise", Path: "capture.csv", SHA256: h, Unit: "V"}}}
	r, e := IngestBench(p, run, dir)
	if e != nil || r.Status != "passed" || math.Abs(r.Measurements[0].Value-math.Sqrt(2)) > 1e-6 {
		t.Fatalf("%+v %v", r, e)
	}
	run.Captures[0].Unit = "mV"
	if _, e = IngestBench(p, run, dir); e == nil {
		t.Fatal("unit mismatch accepted")
	}
	run.Captures[0].Unit = "V"
	put(t, filepath.Join(dir, "capture.csv"), "signal\nNaN\n")
	if _, e = IngestBench(p, run, dir); e == nil {
		t.Fatal("changed capture accepted")
	}
}
func TestPartUnknownFieldsAndDifferences(t *testing.T) {
	a := map[string]any{"mpn": "A", "package": "0603"}
	b := map[string]any{"mpn": "B", "package": "0805"}
	r := CompareParts(a, b)
	if r.Status != "review_required" || len(r.Unknown) == 0 || len(r.Differences) != 1 {
		t.Fatal(r)
	}
}
func releaseFixture(t *testing.T, root string) ReleasePlan {
	put(t, filepath.Join(root, "board.pcb"), "board")
	pcbHash, _ := Hash(filepath.Join(root, "board.pcb"))
	put(t, filepath.Join(root, "review.json"), fmt.Sprintf(`{"pcb_sha256":%q,"checks":{"layout":{"status":"verified","evidence":"fixture"}}}`, pcbHash))
	reviewHash, _ := Hash(filepath.Join(root, "review.json"))
	put(t, filepath.Join(root, "manifest.txt"), fmt.Sprintf("fixture\nSHA-256\n%s  board.pcb\n%s  review.json\n", pcbHash, reviewHash))
	put(t, filepath.Join(root, "build.sh"), "set -eu\nprintf 'fixture package passed\\n'\n")
	return ReleasePlan{Schema: 1, Paths: []string{"board.pcb", "review.json", "manifest.txt", "build.sh"}, Command: []string{"bash", "build.sh"}, Manifest: "manifest.txt", PCB: "board.pcb", Review: "review.json", RequiredChecks: []string{"layout"}}
}
func TestReleaseAtomicFailureReviewAndTamper(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "release")
	p := releaseFixture(t, root)
	if e := BuildRelease(context.Background(), root, p, out); e != nil {
		t.Fatal(e)
	}
	if e := VerifyRelease(out); e != nil {
		t.Fatal(e)
	}
	if e := BuildRelease(context.Background(), root, p, out); e == nil {
		t.Fatal("existing release overwritten")
	}
	put(t, filepath.Join(root, "build.sh"), "exit 9\n")
	bad := filepath.Join(t.TempDir(), "bad")
	if e := BuildRelease(context.Background(), root, p, bad); e == nil {
		t.Fatal("failed build published")
	}
	if _, e := os.Stat(bad); !os.IsNotExist(e) {
		t.Fatal("partial release visible")
	}
	if e := VerifyRelease(out); e != nil {
		t.Fatal("failed build changed old release", e)
	}
	put(t, filepath.Join(out, "board.pcb"), "tampered")
	if e := VerifyRelease(out); e == nil {
		t.Fatal("tamper accepted")
	}
}
func TestReleaseRejectsManifestTraversalAndMissingReview(t *testing.T) {
	root := t.TempDir()
	p := releaseFixture(t, root)
	p.RequiredChecks = append(p.RequiredChecks, "missing")
	if _, e := assembleRelease(root, filepath.Join(t.TempDir(), "out"), p, "key"); e == nil {
		t.Fatal("missing review accepted")
	}
	put(t, filepath.Join(root, "manifest.txt"), "SHA-256\n"+strings.Repeat("a", 64)+"  ../escape\n")
	if _, e := manifestEntries(filepath.Join(root, "manifest.txt")); e == nil {
		t.Fatal("traversal accepted")
	}
}
func TestStrictContracts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	put(t, path, `{"schema":1,"typo":true}`)
	var c BenchPlan
	if e := ReadJSON(path, &c); e == nil {
		t.Fatal("unknown contract field accepted")
	}
	put(t, path, `{} {}`)
	if e := ReadJSON(path, &c); e == nil {
		t.Fatal("trailing JSON accepted")
	}
	if _, e := json.Marshal(c); e != nil {
		t.Fatal(e)
	}
}

func TestReleaseBindsReviewedModelContents(t *testing.T) {
	root := t.TempDir()
	p := releaseFixture(t, root)
	put(t, filepath.Join(root, "model.step"), "model v1")
	h, _ := Hash(filepath.Join(root, "model.step"))
	data, _ := os.ReadFile(filepath.Join(root, "review.json"))
	var review map[string]any
	json.Unmarshal(data, &review)
	review["artifact_sha256"] = map[string]string{"model.step": h}
	encoded, _ := json.Marshal(review)
	put(t, filepath.Join(root, "review.json"), string(encoded))
	rh, _ := Hash(filepath.Join(root, "review.json"))
	bh, _ := Hash(filepath.Join(root, "board.pcb"))
	put(t, filepath.Join(root, "manifest.txt"), fmt.Sprintf("SHA-256\n%s  board.pcb\n%s  review.json\n", bh, rh))
	p.Extras = []string{"model.step"}
	if _, e := assembleRelease(root, filepath.Join(t.TempDir(), "bundle"), p, "key"); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(root, "model.step"), "model v2")
	if _, e := assembleRelease(root, filepath.Join(t.TempDir(), "bundle"), p, "key"); e == nil {
		t.Fatal("changed reviewed model accepted")
	}
}

func TestSourceReaderIgnoresUnrelatedRotationObjects(t *testing.T) {
	p := filepath.Join(t.TempDir(), "circuit.json")
	put(t, p, `[{"type":"cad_component","rotation":{"x":0,"y":0,"z":0}},{"type":"source_component","source_component_id":"s1","name":"J1"},{"type":"pcb_component","source_component_id":"s1","center":{"x":1,"y":2},"width":3,"height":4,"rotation":90,"layer":"top"}]`)
	b, source, e := mechanicalBoard(p)
	if e != nil || !source || len(b.Components) != 1 {
		t.Fatal(b, source, e)
	}
}
