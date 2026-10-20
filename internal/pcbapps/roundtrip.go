package pcbapps

import (
	"flag"
	"fmt"
	"math"
	"github.com/benarent/pcb-tools/internal/pcb"
	"reflect"
	"sort"
	"strings"
)

type IntegrityReport struct {
	Passed bool              `json:"passed"`
	Inputs map[string]string `json:"input_sha256"`
	Checks []string          `json:"checks"`
	Issues []string          `json:"issues"`
	Limits []string          `json:"limits"`
}

func Roundtrip(args []string) error {
	if len(args) == 4 && args[0] == "transfer" {
		return pcb.Transfer(args[1], args[2], args[3])
	}
	f := flag.NewFlagSet("oqroundtrip", flag.ContinueOnError)
	mode := f.String("mode", "check", "check (PCB→PCB), export (PCB→DSN), import (DSN+SES→PCB)")
	ses := f.String("ses", "", "session for import mode")
	tol := f.Float64("tolerance", .002, "coordinate/width tolerance in mm")
	layers := f.String("layers", "", "explicit export copper layers, e.g. F.Cu,B.Cu")
	if e := f.Parse(args); e != nil {
		return e
	}
	if f.NArg() != 3 || *tol <= 0 || *tol > .01 || math.IsNaN(*tol) {
		return fmt.Errorf("usage: oqroundtrip [-mode check|export|import] [-ses session] [-layers F.Cu,B.Cu] before after report.json (0 < tolerance <= .01 mm)")
	}
	a, e := pcb.Read(f.Arg(0))
	if e != nil {
		return e
	}
	b, e := pcb.Read(f.Arg(1))
	if e != nil {
		return e
	}
	r := IntegrityReport{Inputs: map[string]string{f.Arg(0): a.SHA256, f.Arg(1): b.SHA256}, Issues: []string{}, Limits: []string{"Geometry invariants only; final connectivity, DRC and PI/SI remain required.", "Specctra drill dimensions and non-electrical keepout shapes are not compared; mechanical placements are matched geometrically.", "Back-side Specctra placements fail as unsupported; no silent mirroring."}}
	switch *mode {
	case "check":
		if a.Format != "kicad" || b.Format != "kicad" {
			return fmt.Errorf("check requires two KiCad boards")
		}
		r.Issues = append(r.Issues, comparePins(a, b, *tol, true)...)
		r.Issues = append(r.Issues, compareLayers(a.Layers, b.Layers)...)
		r.Issues = append(r.Issues, missingCopper(a.Copper, b.Copper, *tol, true)...)
		r.Checks = []string{"component and pin identity/placement/net/layer", "copper layer set", "baseline tracks and vias preserved"}
	case "export":
		if a.Format != "kicad" || b.Format != "dsn" {
			return fmt.Errorf("export requires KiCad board and DSN")
		}
		r.Issues = append(r.Issues, comparePins(a, b, *tol, true)...)
		expected := a.Layers
		if *layers != "" {
			expected = strings.Split(*layers, ",")
			for _, l := range expected {
				if !contains(a.Layers, l) {
					return fmt.Errorf("requested absent layer %s", l)
				}
			}
		}
		r.Issues = append(r.Issues, compareLayers(expected, b.Layers)...)
		r.Issues = append(r.Issues, missingCopper(a.Copper, b.Copper, *tol, false)...)
		r.Checks = []string{"component and pin identity/placement/net", "explicit copper layers", "all input tracks and vias exported"}
	case "import":
		if a.Format != "dsn" || b.Format != "kicad" || *ses == "" {
			return fmt.Errorf("import requires DSN, KiCad board and -ses")
		}
		s, e := pcb.Read(*ses)
		if e != nil {
			return e
		}
		if s.Format != "ses" {
			return fmt.Errorf("expected SES")
		}
		r.Inputs[*ses] = s.SHA256
		r.Issues = append(r.Issues, comparePins(a, b, *tol, true)...)
		r.Issues = append(r.Issues, comparePins(a, s, *tol, false)...)
		var fixed []pcb.Copper
		for _, c := range a.Copper {
			if c.Protected {
				fixed = append(fixed, c)
			}
		}
		r.Issues = append(r.Issues, missingCopper(fixed, b.Copper, *tol, false)...)
		r.Issues = append(r.Issues, missingCopper(s.Copper, b.Copper, *tol, false)...)
		r.Checks = []string{"DSN and imported pin geometry/nets", "session placement unchanged", "protected DSN copper preserved", "all session copper imported"}
	default:
		return fmt.Errorf("unknown mode %s", *mode)
	}
	sort.Strings(r.Issues)
	r.Passed = len(r.Issues) == 0
	fmt.Printf("routing integrity: %d findings (%s)\n", len(r.Issues), *mode)
	return SaveReport(f.Arg(2), r, r.Issues)
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func compareLayers(a, b []string) []string {
	a = append([]string{}, a...)
	b = append([]string{}, b...)
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		return []string{fmt.Sprintf("copper layers changed: %v -> %v", a, b)}
	}
	return nil
}
func comparePins(a, b pcb.Board, tol float64, pads bool) []string {
	var issues []string
	matched := map[string]bool{}
	for _, ref := range Keys(a.Components) {
		x := a.Components[ref]
		if x.Anonymous && (a.Format != "kicad" || b.Format != "kicad") {
			found := false
			for _, other := range Keys(b.Components) {
				y := b.Components[other]
				numbered := false
				for _, p := range y.Pads {
					if p.Number != "" {
						numbered = true
					}
				}
				if !matched[other] && !numbered && pcb.Distance(x.At, y.At) <= tol && math.Abs(math.Remainder(x.Rotation-y.Rotation, 360)) < .001 {
					matched[other] = true
					found = true
					break
				}
			}
			if !found {
				issues = append(issues, "mechanical placement changed/missing: "+ref)
			}
			continue
		}
		y, ok := b.Components[ref]
		matched[ref] = ok
		if !ok {
			issues = append(issues, "missing component "+ref)
			continue
		}
		if pcb.Distance(x.At, y.At) > tol || math.Abs(math.Remainder(x.Rotation-y.Rotation, 360)) > .001 || x.Layer != y.Layer {
			issues = append(issues, "placement changed: "+ref)
		}
		if !pads {
			continue
		}
		used := make([]bool, len(y.Pads))
		for _, p := range x.Pads {
			if p.Number == "" && (a.Format != "kicad" || b.Format != "kicad") {
				continue
			}
			found := -1
			for i, q := range y.Pads {
				if !used[i] && p.Number == q.Number && p.Net == q.Net && pcb.Distance(p.At, q.At) <= tol {
					found = i
					break
				}
			}
			if found < 0 {
				issues = append(issues, fmt.Sprintf("pin changed/missing: %s.%s net=%q at %.5f,%.5f", ref, p.Number, p.Net, p.At.X, p.At.Y))
			} else {
				used[found] = true
				if a.Format == "kicad" && b.Format == "kicad" {
					q := y.Pads[found]
					if p.Geometry != q.Geometry || p.Kind != q.Kind || p.Shape != q.Shape || pcb.Distance(p.Size, q.Size) > tol || pcb.Distance(p.Drill, q.Drill) > tol || strings.Join(p.Layers, ",") != strings.Join(q.Layers, ",") {
						issues = append(issues, "pad geometry/layers changed: "+ref+"."+p.Number)
					}
				}
			}
		}
		for i, q := range y.Pads {
			if (q.Number != "" || a.Format == "kicad" && b.Format == "kicad") && !used[i] {
				issues = append(issues, "unexpected/changed pin: "+ref+"."+q.Number)
			}
		}
	}
	for ref := range b.Components {
		if matched[ref] {
			continue
		}
		if _, ok := a.Components[ref]; !ok {
			issues = append(issues, "unexpected component "+ref)
		}
	}
	return issues
}

// Match geometric coverage, allowing collinear segment splitting/merging by the router.
func missingCopper(before, after []pcb.Copper, tol float64, drills bool) []string {
	var issues []string
	for _, a := range before {
		found := false
		var intervals [][2]float64
		length := pcb.Distance(a.Start, a.End)
		for _, b := range after {
			if a.Kind != b.Kind || a.Net != b.Net || strings.Join(a.Layers, ",") != strings.Join(b.Layers, ",") || math.Abs(a.Width-b.Width) > tol {
				continue
			}
			if a.Kind == "via" {
				if pcb.Distance(a.Start, b.Start) <= tol && (!drills || math.Abs(a.Drill-b.Drill) <= tol) {
					found = true
					break
				}
				continue
			}
			if a.Kind == "arc" {
				if pcb.Distance(a.Mid, b.Mid) <= tol && ((pcb.Distance(a.Start, b.Start) <= tol && pcb.Distance(a.End, b.End) <= tol) || (pcb.Distance(a.Start, b.End) <= tol && pcb.Distance(a.End, b.Start) <= tol)) {
					found = true
					break
				}
				continue
			}
			if length <= tol {
				if pcb.Distance(a.Start, b.Start) <= tol {
					found = true
					break
				}
				continue
			}
			ux, uy := (a.End.X-a.Start.X)/length, (a.End.Y-a.Start.Y)/length
			perp := func(p pcb.Point) float64 { return math.Abs((p.X-a.Start.X)*uy - (p.Y-a.Start.Y)*ux) }
			if perp(b.Start) > tol || perp(b.End) > tol {
				continue
			}
			project := func(p pcb.Point) float64 { return (p.X-a.Start.X)*ux + (p.Y-a.Start.Y)*uy }
			lo, hi := project(b.Start), project(b.End)
			if lo > hi {
				lo, hi = hi, lo
			}
			intervals = append(intervals, [2]float64{lo, hi})
		}
		if !found && len(intervals) > 0 {
			sort.Slice(intervals, func(i, j int) bool { return intervals[i][0] < intervals[j][0] })
			end := 0.
			for _, v := range intervals {
				if v[0] > end+tol {
					break
				}
				end = math.Max(end, v[1])
			}
			found = end >= length-tol
		}
		if !found {
			issues = append(issues, "missing/changed copper: "+pcb.CopperKey(a))
		}
	}
	return issues
}
