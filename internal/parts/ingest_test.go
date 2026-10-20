package parts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineIngestPortableAtomicAndDated(t *testing.T) {
	dir := t.TempDir()
	circuit, output, catalog := filepath.Join(dir, "circuit.json"), filepath.Join(dir, "parts.json"), filepath.Join(dir, "catalog")
	os.Mkdir(catalog, 0755)
	os.WriteFile(circuit, []byte(`[
		{"type":"source_component","source_component_id":"s1","name":"R1","supplier_part_numbers":{"jlcpcb":["C1"]}},
		{"type":"pcb_component","source_component_id":"s1"},
		{"type":"source_component","source_component_id":"s2","name":"J1","supplier_part_numbers":{"jlcpcb":["C2"]}},
		{"type":"pcb_component","source_component_id":"s2","do_not_place":true},
		{"type":"source_component","source_component_id":"s3","name":"TP1","ftype":"simple_test_point"},
		{"type":"pcb_component","source_component_id":"s3"}
	]`), 0644)
	os.WriteFile(output, []byte(`[{"lcsc":"C1","verified":true,"verified_on":"2020-01-01","datasheet":"reviewed.pdf"},{"lcsc":"C99"}]`), 0644)
	hit := `{"lcsc_id":"C1","name":"10k","package":"R0603","library_type":"basic","stock":100,"price":0.01}`
	saved := filepath.Join(catalog, "C1.json")
	os.WriteFile(saved, []byte(hit), 0644)
	t.Setenv("PATH", "") // Offline must not invoke Node or npx.
	entries, err := Ingest(context.Background(), circuit, output, catalog, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].Verified || *entries[0].VerifiedOn != "2020-01-01" ||
		entries[0].Datasheet != "reviewed.pdf" || len(entries[0].UsedBy) != 1 {
		t.Fatalf("%+v", entries)
	}
	original, _ := os.ReadFile(output)
	for _, invalid := range []string{
		strings.ReplaceAll(hit, `"C1"`, `"C2"`),
		strings.ReplaceAll(hit, `"stock":100`, `"stock":null`),
		strings.ReplaceAll(hit, `"price":0.01`, `"price":-1`),
		strings.ReplaceAll(hit, `"library_type":"basic"`, `"library_type":"unknown"`),
	} {
		os.WriteFile(saved, []byte(invalid), 0644)
		if _, err = Ingest(context.Background(), circuit, output, catalog, true); err == nil {
			t.Fatal("invalid catalog accepted")
		}
		got, _ := os.ReadFile(output)
		if string(got) != string(original) {
			t.Fatal("failed ingest changed parts")
		}
	}
	os.Remove(saved)
	if _, err = Ingest(context.Background(), circuit, output, catalog, true); err == nil || !strings.Contains(err.Error(), "missing offline") {
		t.Fatal(err)
	}
	os.WriteFile(saved, []byte(hit), 0644)
	os.Remove(output)
	entries, err = Ingest(context.Background(), circuit, output, catalog, true)
	if err != nil || entries[0].Verified || entries[0].VerifiedOn != nil {
		t.Fatal(entries, err)
	}
	os.WriteFile(circuit, []byte(`[{"supplier_part_numbers":{"jlcpcb":["../C1"]}}]`), 0644)
	if _, err = Ingest(context.Background(), circuit, output, catalog, true); err == nil {
		t.Fatal("unsafe catalog ID accepted")
	}
}

func TestLiveIngestUsesCatalogBackend(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
test "$1" = "@jlcpcb/cli" && test "$2" = search && test "$3" = C1 || exit 9
printf '%s\n' '{"items":[{"lcsc_id":"C1","name":"10k","package":"0603","library_type":"basic","stock":100,"price":0.01,"unknown_field":"retained"}]}'
`
	os.WriteFile(filepath.Join(dir, "npx"), []byte(script), 0755)
	t.Setenv("PATH", dir)
	circuit := filepath.Join(dir, "circuit.json")
	os.WriteFile(circuit, []byte(`[{"type":"source_component","source_component_id":"s1","name":"R1","supplier_part_numbers":{"jlcpcb":["C1"]}},{"type":"pcb_component","source_component_id":"s1"}]`), 0644)
	entries, err := Ingest(context.Background(), circuit, filepath.Join(dir, "parts.json"), filepath.Join(dir, "catalog"), false)
	if err != nil || len(entries) != 1 || !entries[0].Verified || entries[0].VerifiedOn == nil {
		t.Fatal(entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog", "C1.json"))
	var saved map[string]any
	if err != nil || json.Unmarshal(raw, &saved) != nil || saved["unknown_field"] != "retained" {
		t.Fatal(string(raw), err)
	}
}
