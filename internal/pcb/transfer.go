package pcb

import (
	"fmt"
	"os"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/sexpr"
	"strconv"
	"strings"
)

// Transfer retains the original source footprints and imports every track, arc and via.
// Native KiCad 10's outer-to-outer "buried" via is normalized to a through via.
// Widths are preserved exactly; DRC decides whether the router result is acceptable.
func Transfer(rawPath, routedPath, outPath string) error {
	outputAbs, e := filepath.Abs(outPath)
	if e != nil {
		return e
	}
	for _, input := range []string{rawPath, routedPath} {
		inputAbs, e := filepath.Abs(input)
		if e != nil {
			return e
		}
		same := inputAbs == outputAbs
		ii, ie := os.Stat(input)
		oi, oe := os.Stat(outPath)
		if ie == nil && oe == nil && os.SameFile(ii, oi) {
			same = true
		}
		if same {
			return fmt.Errorf("transfer requires a distinct output")
		}
	}

	raw, e := os.ReadFile(rawPath)
	if e != nil {
		return e
	}
	data, e := os.ReadFile(routedPath)
	if e != nil {
		return e
	}
	base, e := sexpr.Parse(string(raw))
	if e != nil {
		return e
	}
	native, e := sexpr.Parse(string(data))
	if e != nil {
		return e
	}
	if base.Head() != "kicad_pcb" || native.Head() != "kicad_pcb" {
		return fmt.Errorf("transfer needs KiCad files")
	}
	for _, k := range []string{"segment", "arc", "via"} {
		if len(base.Children(k)) > 0 {
			return fmt.Errorf("transfer source already has copper")
		}
	}
	nets := map[string]string{}
	for _, n := range base.Children("net") {
		nets[n.A(2)] = n.A(1)
	}
	if len(nets) == 0 {
		return fmt.Errorf("transfer source needs a numeric net table")
	}
	nativeNets := map[string]string{}
	for _, n := range native.Children("net") {
		nativeNets[n.A(1)] = n.A(2)
	}
	var copper strings.Builder
	count := 0
	for _, kind := range []string{"segment", "arc", "via"} {
		for _, v := range native.Children(kind) {
			net := v.Child("net")
			if net == nil {
				return fmt.Errorf("copper without net")
			}
			name := net.A(1)
			if len(nativeNets) > 0 {
				name = nativeNets[name]
			}
			id, ok := nets[name]
			if !ok || name == "" {
				return fmt.Errorf("unknown/empty copper net %s", name)
			}
			net.List = []*sexpr.Node{{Atom: "net"}, {Atom: id}}
			if kind == "via" && v.A(1) == "buried" {
				l := v.Child("layers")
				if l.A(1) != "F.Cu" || l.A(2) != "B.Cu" {
					return fmt.Errorf("buried via on internal layers requires explicit stackup support")
				}
				v.List = append(v.List[:1], v.List[2:]...)
			}
			copper.WriteString("\n  ")
			copper.WriteString(prettyCopper(v, 1))
			count++
		}
	}
	if count == 0 {
		return fmt.Errorf("no routed copper")
	}
	end := strings.LastIndex(string(raw), ")")
	if end < 0 {
		return fmt.Errorf("invalid source")
	}
	out := string(raw[:end]) + copper.String() + "\n)\n"
	if _, e = Parse(out); e != nil {
		return fmt.Errorf("transfer output invalid: %w", e)
	}
	temp, e := os.CreateTemp(filepath.Dir(outPath), ".oqtransfer-")
	if e != nil {
		return e
	}
	defer os.Remove(temp.Name())
	if _, e = temp.WriteString(out); e != nil {
		temp.Close()
		return e
	}
	if e = temp.Close(); e != nil {
		return e
	}
	if e = os.Rename(temp.Name(), outPath); e != nil {
		return e
	}
	fmt.Printf("Go route transfer: %d copper objects, widths preserved, outer vias normalized\n", count)
	return nil
}
func prettyCopper(n *sexpr.Node, depth int) string {
	if n.List == nil {
		if n.Atom != "" && !strings.ContainsAny(n.Atom, " \t\r\n()\"\\") {
			return n.Atom
		}
		return strconv.Quote(n.Atom)
	}
	var s strings.Builder
	s.WriteByte('(')
	for i, c := range n.List {
		if i > 0 {
			if c.List != nil {
				s.WriteByte('\n')
				s.WriteString(strings.Repeat("  ", depth+1))
			} else {
				s.WriteByte(' ')
			}
		}
		s.WriteString(prettyCopper(c, depth+1))
	}
	s.WriteByte(')')
	return s.String()
}
