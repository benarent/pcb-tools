package pcbapps

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type PartImpact struct {
	Original    string   `json:"original"`
	Candidate   string   `json:"candidate"`
	Status      string   `json:"status"`
	Differences []Change `json:"differences"`
	Unknown     []string `json:"unknown"`
}

// PartCompare consumes explicit engineering facts; absent pin/rating data stays unknown.
func PartCompare(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: oqpart compare original.json candidate.json report.json (one part object per input)")
	}
	a, e := readPart(args[0])
	if e != nil {
		return e
	}
	b, e := readPart(args[1])
	if e != nil {
		return e
	}
	r := CompareParts(a, b)
	fmt.Printf("part substitution: %s (%d differences, %d unknown fields)\n", r.Status, len(r.Differences), len(r.Unknown))
	if e = Write(args[2], r); e != nil {
		return e
	}
	if len(r.Unknown) > 0 || len(r.Differences) > 0 {
		return fmt.Errorf("substitution requires engineering review; no source or BOM changed")
	}
	return nil
}
func readPart(path string) (map[string]any, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var p map[string]any
	if e = json.Unmarshal(data, &p); e != nil {
		return nil, e
	}
	if p == nil || p["mpn"] == nil {
		return nil, fmt.Errorf("part object with mpn required")
	}
	return p, nil
}
func CompareParts(a, b map[string]any) PartImpact {
	r := PartImpact{Original: fmt.Sprint(a["mpn"]), Candidate: fmt.Sprint(b["mpn"]), Status: "facts_match_review_required", Differences: []Change{}, Unknown: []string{}}
	for _, field := range []string{"package", "pinout", "voltage_limits", "tolerance", "temperature_rating", "assembly_requirements", "attributes"} {
		x, xok := a[field]
		y, yok := b[field]
		if !xok || !yok || x == nil || y == nil || mustJSON(x) == `""` || mustJSON(y) == `""` || mustJSON(x) == "{}" || mustJSON(y) == "{}" {
			r.Unknown = append(r.Unknown, field)
			continue
		}
		if mustJSON(x) != mustJSON(y) {
			r.Differences = append(r.Differences, Change{Kind: "part-" + field, Detail: mustJSON(x) + " -> " + mustJSON(y), Gates: strings.Split("BOM,footprint,electrical,assembly", ",")})
		}
	}
	if len(r.Differences) > 0 || len(r.Unknown) > 0 {
		r.Status = "review_required"
	}
	return r
}
