package pcb

import (
	"fmt"
	"github.com/benarent/pcb-tools/internal/sexpr"
	"strings"
)

func scale(n *sexpr.Node, session bool) (float64, error) {
	u := n.Child("unit").A(1)
	res := n.Child("resolution")
	if u == "" {
		u = res.A(1)
	}
	var f float64
	switch u {
	case "mm":
		f = 1
	case "um":
		f = .001
	case "mil":
		f = .0254
	case "inch":
		f = 25.4
	default:
		return 0, fmt.Errorf("unsupported or absent Specctra unit %q", u)
	}
	if session {
		v, e := Number(res.A(2))
		if e != nil || v <= 0 {
			return 0, fmt.Errorf("invalid session resolution")
		}
		f /= v
	}
	return f, nil
}
func specctra(n *sexpr.Node, session bool) (Board, error) {
	b := Board{Format: "dsn", Components: map[string]Component{}}
	if session {
		b.Format = "ses"
	}
	place := n.Child("placement")
	source := n
	if session {
		source = place
	}
	f, e := scale(source, session)
	if e != nil {
		return b, e
	}
	library := n.Child("library")
	images := map[string]*sexpr.Node{}
	for _, v := range library.Children("image") {
		images[v.A(1)] = v
	}
	pinNets := map[string]string{}
	for _, v := range n.Child("network").Children("net") {
		p := v.Child("pins")
		if p != nil {
			for _, a := range p.List[1:] {
				if _, ok := pinNets[a.Atom]; ok {
					return b, fmt.Errorf("duplicate DSN net membership %s", a.Atom)
				}
				pinNets[a.Atom] = v.A(1)
			}
		}
	}
	used := map[string]bool{}
	for _, g := range place.Children("component") {
		for _, v := range g.Children("place") {
			c := Component{Ref: v.A(1), Footprint: g.A(1), Layer: "F.Cu"}
			if v.A(4) != "front" {
				return b, fmt.Errorf("back-side Specctra placement is not supported: %s", c.Ref)
			}
			c.At, e = nums(v, 2, 3)
			if e != nil {
				return b, e
			}
			c.At = Point{c.At.X * f, -c.At.Y * f}
			c.Rotation, e = Number(v.A(5))
			if e != nil {
				return b, e
			}
			c.Rotation = Angle(c.Rotation)
			if _, ok := b.Components[c.Ref]; ok {
				return b, fmt.Errorf("duplicate Specctra placement %s", c.Ref)
			}
			if !session {
				img := images[g.A(1)]
				if img == nil {
					return b, fmt.Errorf("missing DSN image %s", g.A(1))
				}
				for _, p := range img.Children("pin") {
					// Optional (rotate ...) precedes the pin number; collect only atom operands.
					var atoms []string
					for _, a := range p.List[1:] {
						if a.List == nil {
							atoms = append(atoms, a.Atom)
						}
					}
					if len(atoms) != 4 {
						return b, fmt.Errorf("unsupported DSN pin %s", p.String())
					}
					x, er := Number(atoms[2])
					if er != nil {
						return b, er
					}
					y, er := Number(atoms[3])
					if er != nil {
						return b, er
					}
					local := Point{x * f, -y * f}
					id := c.Ref + "-" + atoms[1]
					used[id] = true
					c.Pads = append(c.Pads, Pad{Number: atoms[1], Net: pinNets[id], Local: local, At: Add(c.At, Rotate(local, -c.Rotation))})
				}
			}
			if !session && len(c.Pads) == 0 {
				c.Anonymous = true
			}
			b.Components[c.Ref] = c
		}
	}
	for p := range pinNets {
		if !used[p] {
			return b, fmt.Errorf("DSN network references missing pin %s", p)
		}
	}
	structure := n.Child("structure")
	for _, v := range structure.Children("layer") {
		b.Layers = append(b.Layers, v.A(1))
	}
	wiring := n.Child("wiring")
	if session {
		routes := n.Child("routes")
		f, e = scale(routes, true)
		if e != nil {
			return b, e
		}
		library = routes.Child("library_out")
		wiring = routes.Child("network_out")
	}
	stacks := map[string]*sexpr.Node{}
	for _, s := range library.Children("padstack") {
		stacks[s.A(1)] = s
	}
	var wires func(*sexpr.Node, string) error
	wires = func(parent *sexpr.Node, net string) error {
		for _, v := range parent.Children("wire") {
			p := v.Child("path")
			if p == nil || len(p.List) < 7 || (len(p.List)-3)%2 != 0 {
				return fmt.Errorf("unsupported/invalid Specctra wire")
			}
			name := net
			if name == "" {
				name = v.Child("net").A(1)
			}
			w, e := Number(p.A(2))
			if e != nil || w <= 0 {
				return fmt.Errorf("invalid wire width")
			}
			var prev Point
			for i := 3; i < len(p.List); i += 2 {
				q, e := nums(p, i, i+1)
				if e != nil {
					return e
				}
				q = Point{q.X * f, -q.Y * f}
				if i > 3 {
					b.Copper = append(b.Copper, Copper{Kind: "segment", Net: name, Layers: []string{p.A(1)}, Start: prev, End: q, Width: w * f, Protected: v.Child("type").A(1) == "fix" || v.Child("type").A(1) == "protect"})
				}
				prev = q
			}
		}
		for _, v := range parent.Children("via") {
			stack := stacks[v.A(1)]
			if stack == nil {
				return fmt.Errorf("undefined via padstack %s", v.A(1))
			}
			c := Copper{Kind: "via", Net: net, Protected: v.Child("type").A(1) == "fix" || v.Child("type").A(1) == "protect"}
			if c.Net == "" {
				c.Net = v.Child("net").A(1)
			}
			c.Start, e = nums(v, 2, 3)
			if e != nil {
				return e
			}
			c.Start = Point{c.Start.X * f, -c.Start.Y * f}
			c.End = c.Start
			for _, s := range stack.Children("shape") {
				q := s.Child("circle")
				if q == nil {
					return fmt.Errorf("noncircular via padstack")
				}
				c.Layers = append(c.Layers, q.A(1))
				diam, e := Number(q.A(2))
				if e != nil {
					return e
				}
				if c.Width != 0 && c.Width != diam*f {
					return fmt.Errorf("unequal via layer diameters")
				}
				c.Width = diam * f
			}
			if c.Width <= 0 {
				return fmt.Errorf("empty via padstack")
			}
			b.Copper = append(b.Copper, c)
		}
		return nil
	}
	if session {
		for _, net := range wiring.Children("net") {
			if e := wires(net, net.A(1)); e != nil {
				return b, e
			}
		}
	} else {
		if e := wires(wiring, ""); e != nil {
			return b, e
		}
	}
	if len(b.Components) == 0 {
		return b, fmt.Errorf("no Specctra placements")
	}
	if session {
		seen := map[string]bool{}
		for _, c := range b.Copper {
			for _, l := range c.Layers {
				if !seen[l] {
					b.Layers = append(b.Layers, l)
					seen[l] = true
				}
			}
		}
	}
	for _, l := range b.Layers {
		if !strings.HasSuffix(l, ".Cu") {
			return b, fmt.Errorf("unsupported copper layer %s", l)
		}
	}
	return b, nil
}
