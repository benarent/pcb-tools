package sexpr

import "testing"

func TestQuotedAndSpecctra(t *testing.T) {
	for _, s := range []string{`(pcb (parser (string_quote ")) (net "name with \"quotes\""))`, `(x "" (nested 1 2) "a\\b")`} {
		if _, e := Parse(s); e != nil {
			t.Fatal(e)
		}
	}
	for _, s := range []string{`(x "oops)`, `(x))`, `(x) (y)`, `(x (y)`, `(x ")`} {
		if _, e := Parse(s); e == nil {
			t.Fatalf("accepted malformed %s", s)
		}
	}
}
