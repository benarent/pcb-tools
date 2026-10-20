package pcbapps

import (
	"fmt"
	"math"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/pcb"
	"reflect"
)

type Evidence struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Revision string `json:"revision"`
	Basis    string `json:"basis"`
}
type PadRule struct {
	Number   string     `json:"number"`
	Count    int        `json:"count"`
	Kind     string     `json:"kind,omitempty"`
	Shape    string     `json:"shape,omitempty"`
	Net      *string    `json:"net,omitempty"`
	Local    *pcb.Point `json:"local,omitempty"`
	Size     *pcb.Point `json:"size,omitempty"`
	Drill    *pcb.Point `json:"drill,omitempty"`
	Rotation *float64   `json:"rotation,omitempty"`
	Layers   []string   `json:"layers,omitempty"`
}
type FootprintRule struct {
	Ref       string     `json:"ref"`
	Footprint string     `json:"footprint,omitempty"`
	Value     string     `json:"value,omitempty"`
	PadCount  int        `json:"pad_count"`
	Rotation  *float64   `json:"rotation,omitempty"`
	Evidence  []Evidence `json:"evidence"`
	Pads      []PadRule  `json:"pads"`
}
type FootprintContract struct {
	Schema     int             `json:"schema"`
	Status     string          `json:"status"`
	Tolerance  float64         `json:"tolerance_mm"`
	Footprints []FootprintRule `json:"footprints"`
}
type ContractReport struct {
	Passed      bool     `json:"passed"`
	BoardSHA    string   `json:"board_sha256"`
	ContractSHA string   `json:"contract_sha256"`
	Checks      int      `json:"checks"`
	Issues      []string `json:"issues"`
	Limits      []string `json:"limits"`
}

func verifyEvidence(base string, items []Evidence) error {
	if len(items) == 0 {
		return fmt.Errorf("manufacturer/review evidence is required")
	}
	for _, v := range items {
		if (v.Path == "" || filepath.IsAbs(v.Path)) || v.Revision == "" || v.Basis == "" || len(v.SHA256) != 64 {
			return fmt.Errorf("incomplete evidence entry")
		}
		h, e := Hash(filepath.Join(base, v.Path))
		if e != nil {
			return e
		}
		if h != v.SHA256 {
			return fmt.Errorf("evidence changed: %s", v.Path)
		}
	}
	return nil
}
func Footprint(args []string) error {
	if len(args) != 4 || args[0] != "check" {
		return fmt.Errorf("usage: oqfootprint check contract.json board.kicad_pcb report.json")
	}
	var c FootprintContract
	if e := ReadJSON(args[1], &c); e != nil {
		return e
	}
	b, e := pcb.Read(args[2])
	if e != nil {
		return e
	}
	if b.Format != "kicad" {
		return fmt.Errorf("KiCad board required")
	}
	r, e := CheckFootprints(c, b, filepath.Dir(args[1]))
	if e != nil {
		return e
	}
	r.ContractSHA, e = Hash(args[1])
	if e != nil {
		return e
	}
	fmt.Printf("footprint contracts: %d checks, %d findings\n", r.Checks, len(r.Issues))
	return SaveReport(args[3], r, r.Issues)
}
func CheckFootprints(c FootprintContract, b pcb.Board, base string) (ContractReport, error) {
	r := ContractReport{BoardSHA: b.SHA256, Issues: []string{}, Limits: []string{"Checks only explicitly listed assertions; existing manufacturer audits and assembly-preview review remain required."}}
	if c.Schema != 1 || c.Status != "reviewed" || c.Tolerance <= 0 || c.Tolerance > .05 || len(c.Footprints) == 0 {
		return r, fmt.Errorf("nonempty schema 1 reviewed contract with 0 < tolerance <= .05 mm required")
	}
	seen := map[string]bool{}
	for _, rule := range c.Footprints {
		if rule.Ref == "" || seen[rule.Ref] || rule.PadCount < 1 || len(rule.Pads) == 0 {
			return r, fmt.Errorf("invalid/duplicate footprint rule %q", rule.Ref)
		}
		seen[rule.Ref] = true
		if e := verifyEvidence(base, rule.Evidence); e != nil {
			return r, fmt.Errorf("%s: %w", rule.Ref, e)
		}
		fail := func(ok bool, msg string) {
			r.Checks++
			if !ok {
				r.Issues = append(r.Issues, rule.Ref+": "+msg)
			}
		}
		f, ok := b.Components[rule.Ref]
		fail(ok, "missing footprint")
		if !ok {
			continue
		}
		fail(len(f.Pads) == rule.PadCount, "pad count differs")
		if rule.Footprint != "" {
			fail(f.Footprint == rule.Footprint, "footprint identifier differs")
		}
		if rule.Value != "" {
			fail(f.Value == rule.Value, "part/value differs")
		}
		if rule.Rotation != nil {
			fail(math.Abs(math.Remainder(f.Rotation-*rule.Rotation, 360)) <= .001, "footprint rotation differs")
		}
		for _, p := range rule.Pads {
			if p.Count < 1 {
				return r, fmt.Errorf("pad rule requires positive count: %s.%s", rule.Ref, p.Number)
			}
			var found []pcb.Pad
			for _, v := range f.Pads {
				if v.Number == p.Number {
					found = append(found, v)
				}
			}
			fail(len(found) == p.Count, "pad "+p.Number+" multiplicity differs")
			for _, v := range found {
				prefix := "pad " + p.Number + " "
				if p.Kind != "" {
					fail(v.Kind == p.Kind, prefix+"kind")
				}
				if p.Shape != "" {
					fail(v.Shape == p.Shape, prefix+"shape")
				}
				if p.Net != nil {
					fail(v.Net == *p.Net, prefix+"net/polarity")
				}
				if p.Local != nil {
					fail(pcb.Distance(v.Local, *p.Local) <= c.Tolerance, prefix+"local position")
				}
				if p.Size != nil {
					fail(pcb.Distance(v.Size, *p.Size) <= c.Tolerance, prefix+"land size")
				}
				if p.Drill != nil {
					fail(pcb.Distance(v.Drill, *p.Drill) <= c.Tolerance, prefix+"drill/slot")
				}
				if p.Rotation != nil {
					fail(math.Abs(math.Remainder(v.Rotation-*p.Rotation, 360)) <= .001, prefix+"rotation")
				}
				if len(p.Layers) > 0 {
					fail(reflect.DeepEqual(v.Layers, p.Layers), prefix+"layers")
				}
			}
		}
	}
	r.Passed = len(r.Issues) == 0
	return r, nil
}
