// Package pcb normalizes the geometry used by review tools. Units are mm, KiCad +Y down.
package pcb

import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"github.com/benarent/pcb-tools/internal/sexpr"
	"sort"
	"strconv"
	"strings"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Pad struct {
	Geometry string   `json:"custom_geometry,omitempty"`
	Bounds   []Point  `json:"custom_bounds,omitempty"`
	Number   string   `json:"number"`
	Net      string   `json:"net"`
	Kind     string   `json:"kind"`
	Shape    string   `json:"shape"`
	At       Point    `json:"at"`
	Local    Point    `json:"local"`
	Size     Point    `json:"size"`
	Drill    Point    `json:"drill"`
	Rotation float64  `json:"rotation"`
	Layers   []string `json:"layers"`
}
type Component struct {
	Anonymous bool    `json:"anonymous,omitempty"`
	Ref       string  `json:"ref"`
	Value     string  `json:"value"`
	Footprint string  `json:"footprint"`
	At        Point   `json:"at"`
	Rotation  float64 `json:"rotation"`
	Layer     string  `json:"layer"`
	Pads      []Pad   `json:"pads"`
	Bounds    []Point `json:"bounds,omitempty"`
}
type Copper struct {
	Protected bool     `json:"protected,omitempty"`
	Kind      string   `json:"kind"`
	Net       string   `json:"net"`
	Layers    []string `json:"layers"`
	Start     Point    `json:"start"`
	End       Point    `json:"end"`
	Mid       Point    `json:"mid,omitempty"`
	Width     float64  `json:"width"`
	Drill     float64  `json:"drill,omitempty"`
}
type Board struct {
	Outline    []string             `json:"outline,omitempty"`
	Thickness  float64              `json:"thickness_mm"`
	Format     string               `json:"format"`
	SHA256     string               `json:"sha256"`
	Components map[string]Component `json:"components"`
	Copper     []Copper             `json:"copper"`
	Layers     []string             `json:"layers"`
	Zones      []string             `json:"zones,omitempty"`
}

func Number(s string) (float64, error) {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid finite number %q", s)
	}
	return v, nil
}
func nums(n *sexpr.Node, a, b int) (Point, error) {
	if n == nil {
		return Point{}, fmt.Errorf("missing coordinate")
	}
	x, e := Number(n.A(a))
	if e != nil {
		return Point{}, e
	}
	y, e := Number(n.A(b))
	return Point{x, y}, e
}
func Rotate(p Point, a float64) Point {
	r := a * math.Pi / 180
	return Point{p.X*math.Cos(r) - p.Y*math.Sin(r), p.X*math.Sin(r) + p.Y*math.Cos(r)}
}
func Add(a, b Point) Point        { return Point{a.X + b.X, a.Y + b.Y} }
func Distance(a, b Point) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func Angle(a float64) float64 {
	a = math.Mod(a, 360)
	if a < 0 {
		a += 360
	}
	if math.Abs(a-360) < 1e-8 {
		return 0
	}
	return a
}
func Read(path string) (Board, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return Board{}, e
	}
	b, e := Parse(string(data))
	b.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	return b, e
}
func Parse(text string) (Board, error) {
	n, e := sexpr.Parse(text)
	if e != nil {
		return Board{}, e
	}
	switch n.Head() {
	case "kicad_pcb":
		return kicad(n)
	case "pcb":
		return specctra(n, false)
	case "session":
		return specctra(n, true)
	}
	return Board{}, fmt.Errorf("unsupported board root %s", n.Head())
}
func kicad(n *sexpr.Node) (Board, error) {
	b := Board{Format: "kicad", Components: map[string]Component{}}
	b.Thickness, _ = Number(n.Child("general").Child("thickness").A(1))
	for _, g := range n.List {
		if g.Child("layer").A(1) == "Edge.Cuts" {
			b.Outline = append(b.Outline, semantic(g))
		}
	}
	sort.Strings(b.Outline)
	nets := map[string]string{}
	for _, v := range n.Children("net") {
		nets[v.A(1)] = v.A(2)
	}
	if l := n.Child("layers"); l != nil {
		for _, v := range l.List[1:] {
			if strings.HasSuffix(v.A(1), ".Cu") {
				b.Layers = append(b.Layers, v.A(1))
			}
		}
	}
	net := func(v *sexpr.Node) (string, error) {
		if v == nil {
			return "", nil
		}
		if len(nets) == 0 && len(v.List) == 2 {
			return v.A(1), nil
		}
		s, ok := nets[v.A(1)]
		if !ok {
			return "", fmt.Errorf("undefined net %s", v.A(1))
		}
		return s, nil
	}
	for _, f := range n.Children("footprint") {
		c := Component{Footprint: f.A(1), Layer: f.Child("layer").A(1)}
		for _, p := range f.Children("property") {
			switch p.A(1) {
			case "Reference":
				c.Ref = p.A(2)
			case "Value":
				c.Value = p.A(2)
			}
		}
		for _, p := range f.Children("fp_text") {
			if p.A(1) == "reference" && c.Ref == "" {
				c.Ref = p.A(2)
			}
		}
		if c.Ref == "" {
			c.Anonymous = true
			c.Ref = "@" + f.Child("uuid").A(1)
			if c.Ref == "@" {
				return b, fmt.Errorf("unreferenced footprint lacks UUID")
			}
		}
		if _, ok := b.Components[c.Ref]; ok {
			return b, fmt.Errorf("duplicate reference %s", c.Ref)
		}
		var e error
		c.At, e = nums(f.Child("at"), 1, 2)
		if e != nil {
			return b, e
		}
		if a := f.Child("at").A(3); a != "" {
			c.Rotation, e = Number(a)
			if e != nil {
				return b, e
			}
		}
		c.Rotation = Angle(c.Rotation)
		for _, p := range f.Children("pad") {
			v := Pad{Number: p.A(1), Kind: p.A(2), Shape: p.A(3)}
			v.Local, e = nums(p.Child("at"), 1, 2)
			if e != nil {
				return b, e
			}
			v.At = Add(c.At, Rotate(v.Local, -c.Rotation))
			v.Size, e = nums(p.Child("size"), 1, 2)
			if e != nil {
				return b, e
			}
			v.Rotation = c.Rotation
			if a := p.Child("at").A(3); a != "" {
				v.Rotation, e = Number(a)
				if e != nil {
					return b, e
				}
			}
			v.Net, e = net(p.Child("net"))
			if e != nil {
				return b, e
			}
			if l := p.Child("layers"); l != nil {
				for _, x := range l.List[1:] {
					v.Layers = append(v.Layers, x.Atom)
				}
			}
			if d := p.Child("drill"); d != nil {
				if d.A(1) == "oval" {
					v.Drill, e = nums(d, 2, 3)
				} else {
					v.Drill.X, e = Number(d.A(1))
					v.Drill.Y = v.Drill.X
				}
				if e != nil {
					return b, e
				}
			}
			if prim := p.Child("primitives"); prim != nil {
				v.Geometry = semantic(prim)
				for _, shape := range prim.List[1:] {
					for _, point := range drawingBounds(shape) {
						v.Bounds = append(v.Bounds, Add(v.At, Rotate(point, -v.Rotation)))
					}
				}
			}
			c.Pads = append(c.Pads, v)
		}
		// Courtyards are placement bounds, not proof of physical body dimensions.
		for _, kind := range []string{"fp_line", "fp_rect", "fp_circle", "fp_arc", "fp_poly"} {
			for _, x := range f.Children(kind) {
				if !strings.HasSuffix(x.Child("layer").A(1), ".CrtYd") {
					continue
				}
				for _, p := range drawingBounds(x) {
					c.Bounds = append(c.Bounds, Add(c.At, Rotate(p, -c.Rotation)))
				}
			}
		}
		b.Components[c.Ref] = c
	}
	for _, kind := range []string{"segment", "arc", "via"} {
		for _, s := range n.Children(kind) {
			v := Copper{Kind: kind}
			var e error
			v.Net, e = net(s.Child("net"))
			if e != nil {
				return b, e
			}
			if kind == "via" {
				v.Start, e = nums(s.Child("at"), 1, 2)
				v.End = v.Start
				v.Width, _ = Number(s.Child("size").A(1))
				v.Drill, _ = Number(s.Child("drill").A(1))
				l := s.Child("layers")
				if l == nil {
					return b, fmt.Errorf("via without layers")
				}
				for _, x := range l.List[1:] {
					v.Layers = append(v.Layers, x.Atom)
				}
			} else {
				v.Start, e = nums(s.Child("start"), 1, 2)
				if e != nil {
					return b, e
				}
				v.End, e = nums(s.Child("end"), 1, 2)
				v.Width, _ = Number(s.Child("width").A(1))
				v.Layers = []string{s.Child("layer").A(1)}
				if kind == "arc" {
					v.Mid, e = nums(s.Child("mid"), 1, 2)
				}
			}
			if e != nil || v.Width <= 0 {
				return b, fmt.Errorf("invalid %s geometry", kind)
			}
			b.Copper = append(b.Copper, v)
		}
	}
	for _, z := range n.Children("zone") {
		b.Zones = append(b.Zones, semantic(z))
	}
	sort.Strings(b.Zones)
	if len(b.Components) == 0 || len(b.Layers) == 0 {
		return b, fmt.Errorf("empty board or missing copper layers")
	}
	return b, nil
}

// Remove volatile identities from semantic zone comparisons, keeping fill geometry and rules.
func semantic(n *sexpr.Node) string {
	if n.List == nil {
		return n.String()
	}
	var a []string
	for _, c := range n.List {
		switch c.Head() {
		case "uuid", "tstamp":
			continue
		}
		a = append(a, semantic(c))
	}
	return "(" + strings.Join(a, " ") + ")"
}
func CopperKey(c Copper) string {
	a, z := c.Start, c.End
	if a.X > z.X || a.X == z.X && a.Y > z.Y {
		a, z = z, a
	}
	return fmt.Sprintf("%s|%s|%s|%.5f,%.5f|%.5f,%.5f|%.5f,%.5f|%.5f|%.5f", c.Kind, c.Net, strings.Join(c.Layers, ","), a.X, a.Y, z.X, z.Y, c.Mid.X, c.Mid.Y, c.Width, c.Drill)
}
