package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"github.com/benarent/pcb-tools/internal/precheck"
)

func main() {
	threshold := flag.Float64("long-net-mm", 50, "report pin-to-pin net spans at least this long")
	radius := flag.Float64("ground-radius-mm", 2.5, "maximum distance from required ground pad to reserved via")
	net := flag.String("ground-net", "GND", "ground net name")
	flag.Parse()
	if flag.NArg() != 3 {
		fmt.Fprintln(os.Stderr, "usage: oqprecheck [flags] geometry.json drc.json report.json")
		os.Exit(2)
	}
	var g precheck.Geometry
	var d precheck.DRC
	read(flag.Arg(0), &g)
	read(flag.Arg(1), &d)
	r, err := precheck.CheckWithOptions(g, d, *threshold, *net, *radius)
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		panic(e)
	}
	if e = os.WriteFile(flag.Arg(2), append(b, '\n'), 0644); e != nil {
		panic(e)
	}
	fmt.Printf("pre-route silkscreen: %d collisions\n", len(r.SilkViolations))
	fmt.Printf("ground reservations: %d vias, %d required pad neighborhoods (connectivity still requires final DRC)\n", len(r.GroundVias), r.ReservedPads)
	fmt.Printf("long-net report: %d nets span >=%g mm (pin-to-pin lower bounds, not routed lengths)\n", len(r.LongNets), *threshold)
	if err != nil {
		fmt.Fprintln(os.Stderr, "oqprecheck:", err)
		os.Exit(1)
	}
}
func read(p string, v any) {
	b, e := os.ReadFile(p)
	if e == nil {
		e = json.Unmarshal(b, v)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
