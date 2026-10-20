package precheck

import (
	"fmt"
	"math"
	"sort"
)

type Point struct{ X, Y float64 }
type Pad struct {
	Ref, Pin, Net string
	X, Y          float64
}
type Geometry struct {
	Pads       []Pad
	GroundVias []Point  `json:"ground_vias"`
	Required   []string `json:"required"`
}
type Violation struct{ Type, Severity, Description string }
type DRC struct{ Violations []Violation }
type Span struct {
	Net      string
	SpanMM   float64 `json:"span_mm"`
	From, To string
}
type Report struct {
	SilkViolations []Violation `json:"silk_violations"`
	LongNets       []Span      `json:"long_nets"`
	GroundVias     []Point     `json:"ground_vias"`
	ReservedPads   int         `json:"reserved_pads"`
}

func Check(g Geometry, d DRC, threshold float64) (Report, error) {
	return CheckWithOptions(g, d, threshold, "GND", 2.5)
}

func CheckWithOptions(g Geometry, d DRC, threshold float64, groundNet string, radius float64) (Report, error) {
	r := Report{GroundVias: g.GroundVias, SilkViolations: []Violation{}, LongNets: []Span{}}
	if threshold <= 0 || radius <= 0 || math.IsNaN(threshold) || math.IsNaN(radius) ||
		math.IsInf(threshold, 0) || math.IsInf(radius, 0) || groundNet == "" {
		return r, fmt.Errorf("finite positive span/radius and nonempty ground net required")
	}
	for _, v := range d.Violations {
		if v.Type == "silk_over_copper" || v.Type == "silk_overlap" || v.Type == "silk_over_mask" {
			r.SilkViolations = append(r.SilkViolations, v)
		}
	}
	byNet := map[string][]Pad{}
	byRef := map[string]Pad{}
	for _, p := range g.Pads {
		if p.Net != "" {
			byNet[p.Net] = append(byNet[p.Net], p)
		}
		byRef[p.Ref+"."+p.Pin] = p
	}
	for net, pads := range byNet {
		s := Span{Net: net}
		for i, a := range pads {
			for _, b := range pads[i+1:] {
				v := math.Hypot(a.X-b.X, a.Y-b.Y)
				if v > s.SpanMM {
					s.SpanMM = v
					s.From = a.Ref + "." + a.Pin
					s.To = b.Ref + "." + b.Pin
				}
			}
		}
		if s.SpanMM >= threshold {
			r.LongNets = append(r.LongNets, s)
		}
	}
	sort.Slice(r.LongNets, func(i, j int) bool {
		if r.LongNets[i].SpanMM == r.LongNets[j].SpanMM {
			return r.LongNets[i].Net < r.LongNets[j].Net
		}
		return r.LongNets[i].SpanMM > r.LongNets[j].SpanMM
	})
	for _, ref := range g.Required {
		p, ok := byRef[ref]
		if !ok || p.Net != groundNet {
			return r, fmt.Errorf("reservation anchor %s is missing or not %s", ref, groundNet)
		}
		found := false
		for _, v := range g.GroundVias {
			if math.Hypot(p.X-v.X, p.Y-v.Y) <= radius {
				found = true
				break
			}
		}
		if !found {
			return r, fmt.Errorf("%s has no reserved %s via within %g mm", ref, groundNet, radius)
		}
		r.ReservedPads++
	}
	if len(g.Required) == 0 || len(g.GroundVias) == 0 {
		return r, fmt.Errorf("no ground reservations")
	}
	if len(r.SilkViolations) > 0 {
		return r, fmt.Errorf("%d silkscreen collisions", len(r.SilkViolations))
	}
	return r, nil
}
