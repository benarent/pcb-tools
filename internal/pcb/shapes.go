package pcb

import (
	"math"
	"github.com/benarent/pcb-tools/internal/sexpr"
)

// DrawingBounds yields conservative shape bounds, including rotation-sensitive corners.
func drawingBounds(n *sexpr.Node) []Point {
	kind := n.Head()
	var pts []Point
	get := func(key string) (Point, bool) {
		v := n.Child(key)
		if v == nil {
			return Point{}, false
		}
		p, e := nums(v, 1, 2)
		return p, e == nil
	}
	if p := n.Child("pts"); p != nil {
		for _, v := range p.Children("xy") {
			q, e := nums(v, 1, 2)
			if e == nil {
				pts = append(pts, q)
			}
		}
	}
	a, oka := get("start")
	z, okz := get("end")
	c, okc := get("center")
	switch {
	case kind == "fp_rect" || kind == "gr_rect":
		if oka && okz {
			pts = append(pts, Point{a.X, a.Y}, Point{a.X, z.Y}, Point{z.X, a.Y}, Point{z.X, z.Y})
		}
	case kind == "fp_circle" || kind == "gr_circle":
		if okc && okz {
			r := Distance(c, z)
			pts = append(pts, Point{c.X - r, c.Y - r}, Point{c.X - r, c.Y + r}, Point{c.X + r, c.Y - r}, Point{c.X + r, c.Y + r})
		}
	case kind == "fp_arc" || kind == "gr_arc":
		m, okm := get("mid")
		if oka && okz && okm {
			d := 2 * (a.X*(m.Y-z.Y) + m.X*(z.Y-a.Y) + z.X*(a.Y-m.Y))
			if math.Abs(d) > 1e-12 {
				aa, mm, zz := a.X*a.X+a.Y*a.Y, m.X*m.X+m.Y*m.Y, z.X*z.X+z.Y*z.Y
				center := Point{(aa*(m.Y-z.Y) + mm*(z.Y-a.Y) + zz*(a.Y-m.Y)) / d, (aa*(z.X-m.X) + mm*(a.X-z.X) + zz*(m.X-a.X)) / d}
				r := Distance(center, a)
				pts = append(pts, Point{center.X - r, center.Y - r}, Point{center.X - r, center.Y + r}, Point{center.X + r, center.Y - r}, Point{center.X + r, center.Y + r})
			} else {
				pts = append(pts, a, m, z)
			}
		}
	default:
		if oka {
			pts = append(pts, a)
		}
		if okz {
			pts = append(pts, z)
		}
	}
	w := n.Child("width")
	if w == nil {
		w = n.Child("stroke").Child("width")
	}
	width, _ := Number(w.A(1))
	if width > 0 {
		copyPts := append([]Point{}, pts...)
		for _, p := range copyPts {
			pts = append(pts, Point{p.X - width/2, p.Y - width/2}, Point{p.X + width/2, p.Y + width/2})
		}
	}
	return pts
}
