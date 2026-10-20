package pcb

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKiCadFormatsRotationAndHoles(t *testing.T) {
	for _, net := range []string{`(net 1 "GND")`, `(net "GND")`} {
		table := ""
		if net == `(net 1 "GND")` {
			table = `(net 0 "") (net 1 "GND")`
		}
		s := `(kicad_pcb (layers (0 "F.Cu" signal) (31 "B.Cu" signal)) ` + table + ` (footprint "f" (at 10 20 90) (layer "F.Cu") (property "Reference" "U1") (pad "1" thru_hole oval (at 2 0 90) (size 2 1) (drill oval .8 .4) (layers "*.Cu") ` + net + `)))`
		b, e := Parse(s)
		if e != nil {
			t.Fatal(e)
		}
		p := b.Components["U1"].Pads[0]
		if math.Abs(p.At.X-10) > 1e-8 || math.Abs(p.At.Y-18) > 1e-8 || p.Net != "GND" || p.Drill.Y != .4 {
			t.Fatalf("%+v", p)
		}
	}
}
func TestSpecctraResolutionAndMembership(t *testing.T) {
	dsn := `(pcb demo (parser (string_quote ")) (unit um) (resolution um 10) (structure (layer F.Cu (type signal))) (placement (component f (place U1 10000 -20000 front 90))) (library (image f (pin p 1 2000 0))) (network (net GND (pins U1-1))) (wiring (wire (path F.Cu 200 10000 -18000 12000 -18000) (net GND) (type fix))))`
	b, e := Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	p := b.Components["U1"].Pads[0]
	if Distance(p.At, Point{10, 18}) > 1e-8 || p.Net != "GND" || !b.Copper[0].Protected {
		t.Fatalf("%+v", b)
	}
	ses := `(session demo (placement (resolution um 10) (component f (place U1 100000 -200000 front 90))) (routes (resolution um 10) (library_out) (network_out (net GND (wire (path F.Cu 2000 100000 -180000 120000 -180000))))))`
	s, e := Parse(ses)
	if e != nil {
		t.Fatal(e)
	}
	if Distance(s.Copper[0].Start, b.Copper[0].Start) > 1e-8 || s.Copper[0].Width != b.Copper[0].Width {
		t.Fatalf("SES units differ: %+v", s)
	}
}

func TestTransferIncludesBuriedViasAndPreservesNarrowTracks(t *testing.T) {
	dir := t.TempDir()
	raw := `(kicad_pcb (layers (0 F.Cu signal) (31 B.Cu signal)) (net 0 "") (net 1 "GND") (footprint f (layer F.Cu) (at 0 0) (property "Reference" "J1") (pad "1" smd rect (at 0 0) (size 1 1) (layers F.Cu) (net 1 "GND"))))`
	native := `(kicad_pcb (via buried (at 1 1) (size .6) (drill .3) (layers F.Cu B.Cu) (net "GND")) (segment (start 0 0) (end 1 1) (width .12) (layer F.Cu) (net "GND")))`
	a, b, out := filepath.Join(dir, "raw"), filepath.Join(dir, "native"), filepath.Join(dir, "output")
	os.WriteFile(a, []byte(raw), 0644)
	os.WriteFile(b, []byte(native), 0644)
	alias := filepath.Join(dir, "alias")
	if e := os.Symlink(a, alias); e != nil {
		t.Fatal(e)
	}
	if e := Transfer(a, b, alias); e == nil {
		t.Fatal("input alias overwritten")
	}
	if e := Transfer(a, b, out); e != nil {
		t.Fatal(e)
	}
	r, e := Read(out)
	if e != nil || len(r.Copper) != 2 {
		t.Fatal(r, e)
	}
	if r.Copper[0].Width != .12 || r.Copper[1].Kind != "via" {
		t.Fatal(r.Copper)
	}
	data, _ := os.ReadFile(out)
	if strings.Contains(string(data), "via buried") {
		t.Fatal("outer via remains buried")
	}
}
