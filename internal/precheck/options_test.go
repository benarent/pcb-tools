package precheck

import "testing"

func TestCustomGroundAndThreshold(t *testing.T) {
	g := Geometry{Pads: []Pad{{Ref: "U1", Pin: "1", Net: "AGND", X: 0, Y: 0}, {Ref: "U2", Pin: "1", Net: "AGND", X: 20, Y: 0}},
		GroundVias: []Point{{X: 1, Y: 0}}, Required: []string{"U1.1"}}
	r, err := CheckWithOptions(g, DRC{}, 10, "AGND", 1.5)
	if err != nil || len(r.LongNets) != 1 || r.ReservedPads != 1 {
		t.Fatal(r, err)
	}
	if _, err = CheckWithOptions(g, DRC{}, 10, "AGND", 0.5); err == nil {
		t.Fatal("accepted out-of-range via")
	}
	if _, err = CheckWithOptions(g, DRC{}, 0, "AGND", 1.5); err == nil {
		t.Fatal("accepted invalid threshold")
	}
}
