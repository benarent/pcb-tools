// Package sexpr reads KiCad and Specctra s-expressions without regexp block matching.
package sexpr

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

type Node struct {
	Atom string
	List []*Node
}

func (n *Node) Head() string { return n.A(0) }
func (n *Node) A(i int) string {
	if n == nil || i < 0 || i >= len(n.List) {
		return ""
	}
	return n.List[i].Atom
}
func (n *Node) Child(key string) *Node {
	if n != nil {
		for _, v := range n.List {
			if v.Head() == key {
				return v
			}
		}
	}
	return nil
}
func (n *Node) Children(key string) []*Node {
	var out []*Node
	if n != nil {
		for _, v := range n.List {
			if v.Head() == key {
				out = append(out, v)
			}
		}
	}
	return out
}
func (n *Node) String() string {
	if n.List == nil {
		return strconv.Quote(n.Atom)
	}
	a := make([]string, len(n.List))
	for i, c := range n.List {
		a[i] = c.String()
	}
	return "(" + strings.Join(a, " ") + ")"
}
func Parse(s string) (*Node, error) {
	r := []rune(s)
	i := 0
	var read func(int) (*Node, error)
	space := func() {
		for i < len(r) && unicode.IsSpace(r[i]) {
			i++
		}
	}
	read = func(depth int) (*Node, error) {
		space()
		if i >= len(r) {
			return nil, fmt.Errorf("unexpected end of expression")
		}
		if depth > 256 {
			return nil, fmt.Errorf("expression too deep")
		}
		if r[i] == '(' {
			i++
			n := &Node{List: []*Node{}}
			for {
				space()
				if i >= len(r) {
					return nil, fmt.Errorf("unclosed expression")
				}
				if r[i] == ')' {
					i++
					return n, nil
				}
				if n.A(0) == "string_quote" && len(n.List) == 1 && r[i] == '"' && i+1 < len(r) && r[i+1] == ')' {
					i++
					n.List = append(n.List, &Node{Atom: "\""})
					continue
				}
				c, e := read(depth + 1)
				if e != nil {
					return nil, e
				}
				n.List = append(n.List, c)
			}
		}
		if r[i] == ')' {
			return nil, fmt.Errorf("unexpected closing parenthesis")
		}
		if r[i] == '"' {
			i++
			start := i
			// Specctra's literal (string_quote ") is the sole unpaired quote syntax.

			var b strings.Builder
			for i < len(r) {
				c := r[i]
				i++
				if c == '"' {
					return &Node{Atom: b.String()}, nil
				}
				if c == '\\' {
					if i >= len(r) {
						break
					}
					c = r[i]
					i++
					switch c {
					case 'n':
						c = '\n'
					case 'r':
						c = '\r'
					case 't':
						c = '\t'
					}
				}
				b.WriteRune(c)
			}
			return nil, fmt.Errorf("unterminated quoted string at %d", start)
		}
		start := i
		for i < len(r) && !unicode.IsSpace(r[i]) && r[i] != '(' && r[i] != ')' {
			i++
		}
		return &Node{Atom: string(r[start:i])}, nil
	}
	n, e := read(0)
	if e != nil {
		return nil, e
	}
	space()
	if i != len(r) {
		return nil, fmt.Errorf("trailing expression at %d", i)
	}
	return n, nil
}
