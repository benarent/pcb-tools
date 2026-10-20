// oqpart uses the existing catalog contract; it never selects replacement parts.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"github.com/benarent/pcb-tools/internal/parts"
	"github.com/benarent/pcb-tools/internal/pcbapps"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "oqpart:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: oqpart search query | oqpart ingest circuit.json parts.json catalog-dir [--offline] | oqpart compare baseline.json candidate.json report.json")
	}
	switch os.Args[1] {
	case "compare":
		return pcbapps.PartCompare(os.Args[2:])
	case "search":
		if len(os.Args) != 3 || os.Args[2] == "" {
			return fmt.Errorf("search requires one query")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "npx", "@jlcpcb/cli", "search", os.Args[2], "--json", "--limit", "10")
		cmd.Stderr = os.Stderr
		b, err := cmd.Output()
		if err != nil {
			return err
		}
		if !json.Valid(b) {
			return fmt.Errorf("catalog returned invalid JSON")
		}
		os.Stdout.Write(b)
		return nil
	case "ingest":
		if len(os.Args) != 5 && !(len(os.Args) == 6 && os.Args[5] == "--offline") {
			return fmt.Errorf("ingest requires circuit, parts, catalog and optional --offline")
		}
		entries, err := parts.Ingest(context.Background(), os.Args[2], os.Args[3], os.Args[4], len(os.Args) == 6)
		if err != nil {
			return err
		}
		basic, placements := 0, 0
		for _, entry := range entries {
			if entry.JLCType == "Basic" {
				basic++
			}
			placements += len(entry.UsedBy)
		}
		fmt.Printf("parts catalog: %d types (%d Basic / %d Extended), %d placements\n", len(entries), basic, len(entries)-basic, placements)
		return nil
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
