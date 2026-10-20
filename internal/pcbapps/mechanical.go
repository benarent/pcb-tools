package pcbapps

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/pcb"
)

type Body struct {
	Ref    string    `json:"ref"`
	Size   pcb.Point `json:"size"`
	Offset pcb.Point `json:"offset"`
	Height float64   `json:"height_mm"`
	Side   string    `json:"side"`
}
type Envelope struct {
	ID       string     `json:"id"`
	Anchor   string     `json:"anchor,omitempty"`
	Center   pcb.Point  `json:"center"`
	Size     pcb.Point  `json:"size"`
	Rotation float64    `json:"rotation"`
	Side     string     `json:"side"`
	ZMin     float64    `json:"z_min_mm"`
	ZMax     *float64   `json:"z_max_mm,omitempty"`
	Margin   float64    `json:"margin_mm"`
	Allow    []string   `json:"allow"`
	Evidence []Evidence `json:"evidence"`
}
type MechanicalContract struct {
	Schema           int        `json:"schema"`
	CoordinateSystem string     `json:"coordinate_system"`
	Bodies           []Body     `json:"bodies"`
	Envelopes        []Envelope `json:"envelopes"`
}
type MechReport struct {
	ContractReport
	BoundsBasis map[string]string `json:"bounds_basis"`
}
type rect struct {
	center, size pcb.Point
	angle        float64
}

func corners(r rect) []pcb.Point {
	var p []pcb.Point
	for _, v := range []pcb.Point{{X: -1, Y: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}} {
		p = append(p, pcb.Add(r.center, pcb.Rotate(pcb.Point{X: v.X * r.size.X / 2, Y: v.Y * r.size.Y / 2}, r.angle)))
	}
	return p
}
func overlap(a, b rect) bool {
	pa, pb := corners(a), corners(b)
	for _, r := range []rect{a, b} {
		for _, axis := range []pcb.Point{pcb.Rotate(pcb.Point{X: 1, Y: 0}, r.angle), pcb.Rotate(pcb.Point{X: 0, Y: 1}, r.angle)} {
			rangeOf := func(p []pcb.Point) (float64, float64) {
				lo, hi := math.Inf(1), math.Inf(-1)
				for _, v := range p {
					d := v.X*axis.X + v.Y*axis.Y
					lo = math.Min(lo, d)
					hi = math.Max(hi, d)
				}
				return lo, hi
			}
			alo, ahi := rangeOf(pa)
			blo, bhi := rangeOf(pb)
			if ahi <= blo || bhi <= alo {
				return false
			}
		}
	}
	return true
}
func Mechanical(args []string) error {
	if len(args) != 4 || args[0] != "check" {
		return fmt.Errorf("usage: oqmechanical check contract.json board.kicad_pcb|source.circuit.json report.json")
	}
	var c MechanicalContract
	if e := ReadJSON(args[1], &c); e != nil {
		return e
	}
	b, source, e := mechanicalBoard(args[2])
	if e != nil {
		return e
	}
	r, e := CheckMechanical(c, b, filepath.Dir(args[1]), source)
	if e != nil {
		return e
	}
	r.ContractSHA, e = Hash(args[1])
	if e != nil {
		return e
	}
	fmt.Printf("mechanical envelopes: %d checks, %d findings\n", r.Checks, len(r.Issues))
	return SaveReport(args[3], r, r.Issues)
}
func mechanicalBoard(path string) (pcb.Board, bool, error) {
	if filepath.Ext(path) != ".json" {
		b, e := pcb.Read(path)
		return b, false, e
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return pcb.Board{}, true, e
	}
	type sourceRow struct {
		Type     string    `json:"type"`
		ID       string    `json:"source_component_id"`
		Name     string    `json:"name"`
		Center   pcb.Point `json:"center"`
		Rotation float64   `json:"rotation"`
		Width    float64   `json:"width"`
		Height   float64   `json:"height"`
		Layer    string    `json:"layer"`
	}
	var raw []json.RawMessage
	if e = json.Unmarshal(data, &raw); e != nil {
		return pcb.Board{}, true, e
	}
	var rows []sourceRow
	for _, r := range raw {
		var kind struct {
			Type string `json:"type"`
		}
		if e = json.Unmarshal(r, &kind); e != nil {
			return pcb.Board{}, true, e
		}
		if kind.Type != "source_component" && kind.Type != "pcb_component" {
			continue
		}
		var row sourceRow
		if e = json.Unmarshal(r, &row); e != nil {
			return pcb.Board{}, true, e
		}
		rows = append(rows, row)
	}
	names := map[string]string{}
	for _, v := range rows {
		if v.Type == "source_component" {
			names[v.ID] = v.Name
		}
	}
	b := pcb.Board{Format: "source", Components: map[string]pcb.Component{}}
	b.SHA256, e = Hash(path)
	if e != nil {
		return b, true, e
	}
	// Source uses +Y up and an origin at the board center. Contract stays in that frame.
	for _, v := range rows {
		if v.Type != "pcb_component" {
			continue
		}
		ref := names[v.ID]
		if ref == "" || v.Width <= 0 || v.Height <= 0 {
			return b, true, fmt.Errorf("invalid source component geometry")
		}
		if _, ok := b.Components[ref]; ok {
			return b, true, fmt.Errorf("duplicate source reference")
		}
		layer := "F.Cu"
		if v.Layer == "bottom" {
			layer = "B.Cu"
		} else if v.Layer != "top" {
			return b, true, fmt.Errorf("unknown source side %s", v.Layer)
		}
		b.Components[ref] = pcb.Component{Ref: ref, At: v.Center, Rotation: v.Rotation, Layer: layer, Bounds: corners(rect{v.Center, pcb.Point{X: v.Width, Y: v.Height}, 0})}
	}
	if len(b.Components) == 0 {
		return b, true, fmt.Errorf("empty source geometry")
	}
	return b, true, nil
}
func CheckMechanical(c MechanicalContract, b pcb.Board, base string, source bool) (MechReport, error) {
	r := MechReport{ContractReport: ContractReport{BoardSHA: b.SHA256, Issues: []string{}, Limits: []string{"Rectangular access/body envelope screening, not a 3D solid collision solver or physical fit qualification.", "Explicit body sizes are required for height-limited checks; bounds basis is reported for every component."}}, BoundsBasis: map[string]string{}}
	expected := "kicad-mm-y-down"
	if source {
		expected = "source-mm-y-up"
	}
	if c.Schema != 1 || c.CoordinateSystem != expected || len(c.Envelopes) == 0 {
		return r, fmt.Errorf("schema 1 nonempty envelopes in %s coordinates required", expected)
	}
	bodies := map[string]Body{}
	for _, v := range c.Bodies {
		if v.Size.X <= 0 || v.Size.Y <= 0 || v.Height < 0 || bodies[v.Ref].Ref != "" {
			return r, fmt.Errorf("invalid body %s", v.Ref)
		}
		if _, ok := b.Components[v.Ref]; !ok {
			return r, fmt.Errorf("body references absent component %s", v.Ref)
		}
		bodies[v.Ref] = v
	}
	geometry := map[string]rect{}
	for _, ref := range Keys(b.Components) {
		v := b.Components[ref]
		angle := -v.Rotation
		if source {
			angle = v.Rotation
		}
		if body, ok := bodies[ref]; ok {
			geometry[ref] = rect{pcb.Add(v.At, pcb.Rotate(body.Offset, angle)), body.Size, angle}
			r.BoundsBasis[ref] = "explicit body"
			continue
		}
		pts := append([]pcb.Point{}, v.Bounds...)
		basis := "courtyard bounding box"
		if source {
			basis = "source placement bounds"
		}
		if len(pts) == 0 {
			basis = "pad bounds (body size unknown)"
			for _, p := range v.Pads {
				pts = append(pts, corners(rect{p.At, p.Size, -p.Rotation})...)
				pts = append(pts, p.Bounds...)
			}
		}
		if len(pts) == 0 {
			return r, fmt.Errorf("no geometry for %s", ref)
		}
		lo, hi := pcb.Point{X: math.Inf(1), Y: math.Inf(1)}, pcb.Point{X: math.Inf(-1), Y: math.Inf(-1)}
		for _, p := range pts {
			lo.X = math.Min(lo.X, p.X)
			lo.Y = math.Min(lo.Y, p.Y)
			hi.X = math.Max(hi.X, p.X)
			hi.Y = math.Max(hi.Y, p.Y)
		}
		geometry[ref] = rect{pcb.Point{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2}, pcb.Point{X: hi.X - lo.X, Y: hi.Y - lo.Y}, 0}
		r.BoundsBasis[ref] = basis
	}
	seen := map[string]bool{}
	for _, env := range c.Envelopes {
		if env.ID == "" || seen[env.ID] || env.Size.X <= 0 || env.Size.Y <= 0 || env.Margin < 0 || env.ZMin < 0 || (env.ZMax != nil && *env.ZMax <= env.ZMin) || (env.Side != "top" && env.Side != "bottom" && env.Side != "both") {
			return r, fmt.Errorf("invalid envelope %s", env.ID)
		}
		seen[env.ID] = true
		if e := verifyEvidence(base, env.Evidence); e != nil {
			return r, e
		}
		area := rect{env.Center, pcb.Point{X: env.Size.X + 2*env.Margin, Y: env.Size.Y + 2*env.Margin}, env.Rotation}
		if env.Anchor != "" {
			anchor, ok := b.Components[env.Anchor]
			if !ok {
				return r, fmt.Errorf("missing envelope anchor %s", env.Anchor)
			}
			angle := -anchor.Rotation
			if source {
				angle = anchor.Rotation
			}
			area.center = pcb.Add(anchor.At, pcb.Rotate(env.Center, angle))
			area.angle += angle
		}
		for _, ref := range env.Allow {
			if _, ok := b.Components[ref]; !ok {
				return r, fmt.Errorf("unknown allowed reference %s", ref)
			}
		}
		for _, ref := range Keys(geometry) {
			if contains(env.Allow, ref) {
				continue
			}
			v := b.Components[ref]
			side := "top"
			if v.Layer == "B.Cu" {
				side = "bottom"
			}
			if env.Side != "both" && env.Side != side {
				continue
			}
			r.Checks++
			if !overlap(area, geometry[ref]) {
				continue
			}
			if env.ZMin > 0 || env.ZMax != nil {
				body, ok := bodies[ref]
				if !ok {
					r.Issues = append(r.Issues, env.ID+": unknown body height for "+ref)
					continue
				}
				if body.Height <= env.ZMin {
					continue
				}
			}
			r.Issues = append(r.Issues, env.ID+": obstructed by "+ref+" ("+r.BoundsBasis[ref]+")")
		}
	}
	r.Passed = len(r.Issues) == 0
	return r, nil
}
