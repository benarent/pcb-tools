package pcbapps

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"math"
	"os"
	"github.com/benarent/pcb-tools/internal/pcb"
	"reflect"
	"strings"
)

type Change struct {
	Kind   string   `json:"kind"`
	Ref    string   `json:"ref,omitempty"`
	Detail string   `json:"detail"`
	Gates  []string `json:"affected_gates"`
}
type DiffReport struct {
	Before        string   `json:"before_sha256"`
	After         string   `json:"after_sha256"`
	Changes       []Change `json:"changes"`
	AddedCopper   int      `json:"added_copper"`
	RemovedCopper int      `json:"removed_copper"`
	Overlay       string   `json:"overlay,omitempty"`
}

func Diff(args []string) error {
	f := flag.NewFlagSet("oqdiff", flag.ContinueOnError)
	svg := f.String("svg", "", "SVG overlay output")
	bomA := f.String("bom-before", "", "previous parts JSON")
	bomB := f.String("bom-after", "", "new parts JSON")
	drcA := f.String("drc-before", "", "previous DRC JSON")
	drcB := f.String("drc-after", "", "new DRC JSON")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 3 {
		return fmt.Errorf("usage: oqdiff [-svg overlay.svg] [-bom-before a -bom-after b] [-drc-before a -drc-after b] before.kicad_pcb after.kicad_pcb report.json")
	}
	a, e := pcb.Read(f.Arg(0))
	if e != nil {
		return e
	}
	b, e := pcb.Read(f.Arg(1))
	if e != nil {
		return e
	}
	if a.Format != "kicad" || b.Format != "kicad" {
		return fmt.Errorf("diff requires KiCad boards")
	}
	r := DesignDiff(a, b)
	r.Overlay = *svg
	for _, pair := range [][3]string{{*bomA, *bomB, "bom"}, {*drcA, *drcB, "drc"}} {
		if (pair[0] == "") != (pair[1] == "") {
			return fmt.Errorf("both %s inputs required", pair[2])
		}
		if pair[0] == "" {
			continue
		}
		c, e := jsonChanges(pair[0], pair[1], pair[2])
		if e != nil {
			return e
		}
		r.Changes = append(r.Changes, c...)
	}
	if *svg != "" {
		if e = os.WriteFile(*svg, []byte(Overlay(a, b)), 0644); e != nil {
			return e
		}
	}
	fmt.Printf("design diff: %d changes, %d copper added / %d removed\n", len(r.Changes), r.AddedCopper, r.RemovedCopper)
	return Write(f.Arg(2), r)
}
func DesignDiff(a, b pcb.Board) DiffReport {
	r := DiffReport{Before: a.SHA256, After: b.SHA256, Changes: []Change{}}
	add := func(kind, ref, detail string, gates ...string) {
		r.Changes = append(r.Changes, Change{kind, ref, detail, gates})
	}
	for _, ref := range Keys(a.Components) {
		x := a.Components[ref]
		y, ok := b.Components[ref]
		if !ok {
			add("removed", ref, x.Value, "source", "netlist", "assembly")
			continue
		}
		if pcb.Distance(x.At, y.At) > .00001 || math.Abs(x.Rotation-y.Rotation) > .00001 || x.Layer != y.Layer {
			add("placement", ref, fmt.Sprintf("(%.4f,%.4f) %.2f° %s -> (%.4f,%.4f) %.2f° %s", x.At.X, x.At.Y, x.Rotation, x.Layer, y.At.X, y.At.Y, y.Rotation, y.Layer), "mechanical", "routing", "DRC", "CPL", "layout-review")
		}
		if x.Value != y.Value || x.Footprint != y.Footprint {
			add("part", ref, x.Value+" / "+x.Footprint+" -> "+y.Value+" / "+y.Footprint, "BOM", "footprint", "electrical", "assembly")
		}
		for _, issue := range comparePins(pcb.Board{Format: "kicad", Components: map[string]pcb.Component{ref: x}}, pcb.Board{Format: "kicad", Components: map[string]pcb.Component{ref: y}}, .00001, true) {
			if !strings.HasPrefix(issue, "placement") {
				add("pin", ref, issue, "netlist", "footprint", "routing", "DRC")
			}
		}
	}
	for _, ref := range Keys(b.Components) {
		if _, ok := a.Components[ref]; !ok {
			add("added", ref, b.Components[ref].Value, "source", "netlist", "assembly")
		}
	}
	count := map[string]int{}
	for _, c := range a.Copper {
		count[pcb.CopperKey(c)]--
	}
	for _, c := range b.Copper {
		count[pcb.CopperKey(c)]++
	}
	for _, k := range Keys(count) {
		v := count[k]
		if v > 0 {
			r.AddedCopper += v
		} else {
			r.RemovedCopper -= v
		}
		if v != 0 {
			add("copper", "", fmt.Sprintf("%+d %s", v, k), "DRC", "PI", "SI", "layout-review")
		}
	}
	if !reflect.DeepEqual(a.Layers, b.Layers) {
		add("layers", "", fmt.Sprintf("%v -> %v", a.Layers, b.Layers), "stackup", "DRC", "PI", "SI")
	}
	if a.Thickness != b.Thickness || !reflect.DeepEqual(a.Outline, b.Outline) {
		add("mechanical", "", "board thickness/outline changed", "mechanical", "DRC", "layout-review")
	}
	if !reflect.DeepEqual(a.Zones, b.Zones) {
		add("zones", "", "zone definitions/fills changed", "DRC", "PI", "SI", "layout-review")
	}
	return r
}
func jsonChanges(a, b, kind string) ([]Change, error) {
	read := func(p string) (map[string]json.RawMessage, error) {
		data, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		m := map[string]json.RawMessage{}
		if kind == "bom" {
			var rows []map[string]json.RawMessage
			if e = json.Unmarshal(data, &rows); e != nil {
				return nil, e
			}
			for _, row := range rows {
				var key string
				for _, field := range []string{"lcsc", "ref", "name"} {
					if v := row[field]; v != nil {
						json.Unmarshal(v, &key)
						break
					}
				}
				if key == "" {
					return nil, fmt.Errorf("BOM row missing lcsc/ref/name")
				}
				if _, ok := m[key]; ok {
					return nil, fmt.Errorf("duplicate BOM key %s", key)
				}
				m[key], _ = json.Marshal(row)
			}
		} else {
			var d map[string]json.RawMessage
			if e = json.Unmarshal(data, &d); e != nil {
				return nil, e
			}
			for _, field := range []string{"violations", "unconnected_items", "footprint_errors", "schematic_parity"} {
				var rows []map[string]any
				if v := d[field]; v != nil {
					if e = json.Unmarshal(v, &rows); e != nil {
						return nil, e
					}
				}
				for _, row := range rows {
					delete(row, "uuid")
					v, _ := json.Marshal(row)
					m[string(v)] = v
				}
			}
		}
		return m, nil
	}
	x, e := read(a)
	if e != nil {
		return nil, e
	}
	y, e := read(b)
	if e != nil {
		return nil, e
	}
	var out []Change
	for _, k := range Keys(x) {
		v, ok := y[k]
		if !ok || string(x[k]) != string(v) {
			out = append(out, Change{Kind: kind, Detail: "removed/changed: " + k + " " + string(x[k]), Gates: []string{kind}})
		}
	}
	for _, k := range Keys(y) {
		if v, ok := x[k]; !ok || string(v) != string(y[k]) {
			out = append(out, Change{Kind: kind, Detail: "added/changed: " + k + " " + string(y[k]), Gates: []string{kind}})
		}
	}
	return out, nil
}
func Overlay(a, b pcb.Board) string {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, board := range []pcb.Board{a, b} {
		for _, c := range board.Components {
			minX = math.Min(minX, c.At.X-5)
			maxX = math.Max(maxX, c.At.X+5)
			minY = math.Min(minY, c.At.Y-5)
			maxY = math.Max(maxY, c.At.Y+5)
		}
	}
	var s strings.Builder
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%f %f %f %f"><title>Board change overlay: red before, blue after</title><rect x="%f" y="%f" width="%f" height="%f" fill="white"/>`, minX, minY-5, maxX-minX, maxY-minY+5, minX, minY-5, maxX-minX, maxY-minY+5)
	fmt.Fprintf(&s, `<text x="%f" y="%f" font-size="2">Red: before · Blue: after · Pads/tracks only; zones excluded</text>`, minX, minY-2)
	for i, board := range []pcb.Board{a, b} {
		color := []string{"#d43b42", "#1677cb"}[i]
		fmt.Fprintf(&s, `<g stroke="%s" fill="%s" opacity="0.5">`, color, color)
		for _, c := range board.Copper {
			if c.Kind == "segment" {
				fmt.Fprintf(&s, `<path d="M %f %f L %f %f" fill="none" stroke-width="%f"/>`, c.Start.X, c.Start.Y, c.End.X, c.End.Y, c.Width)
			} else if c.Kind == "via" {
				fmt.Fprintf(&s, `<circle cx="%f" cy="%f" r="%f" fill="none" stroke-width=".15"/>`, c.Start.X, c.Start.Y, c.Width/2)
			}
		}
		for _, ref := range Keys(board.Components) {
			c := board.Components[ref]
			for _, p := range c.Pads {
				fmt.Fprintf(&s, `<rect x="%f" y="%f" width="%f" height="%f" transform="rotate(%f %f %f)" stroke-width=".05"/>`, p.At.X-p.Size.X/2, p.At.Y-p.Size.Y/2, p.Size.X, p.Size.Y, -p.Rotation, p.At.X, p.At.Y)
			}
			fmt.Fprintf(&s, `<text x="%f" y="%f" font-size="1.1" stroke="none">%s</text>`, c.At.X, c.At.Y, html.EscapeString(ref))
		}
		s.WriteString("</g>")
	}
	s.WriteString("</svg>\n")
	return s.String()
}
