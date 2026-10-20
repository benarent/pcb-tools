package precheck

import "testing"

func TestRejectSilkAndMissingOrWrongGround(t *testing.T) {
	g := Geometry{Pads: []Pad{{Ref: "C1", Pin: "2", Net: "GND", X: 1, Y: 2}, {Ref: "J1", Pin: "1", Net: "SIGNAL"}, {Ref: "U1", Pin: "1", Net: "SIGNAL", X: 60}}, GroundVias: []Point{{1, 2}}, Required: []string{"C1.2"}}
	r, err := Check(g, DRC{}, 50)
	if err != nil || r.ReservedPads != 1 || len(r.LongNets) != 1 || r.LongNets[0].SpanMM != 60 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err = Check(g, DRC{Violations: []Violation{{Type: "silk_over_copper", Severity: "warning"}}}, 50); err == nil {
		t.Fatal("silk collision accepted")
	}
	g.Pads[0].Net = "V3D"
	if _, err = Check(g, DRC{}, 50); err == nil {
		t.Fatal("wrong-net reservation accepted")
	}
	g.Pads[0].Net = "GND"
	g.GroundVias = []Point{{20, 20}}
	if _, err = Check(g, DRC{}, 50); err == nil {
		t.Fatal("missing local via accepted")
	}
}
