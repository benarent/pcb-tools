// Package parts maps placed circuit-json parts to saved or live JLC catalog data.
package parts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/benarent/pcb-tools/internal/cache"
)

type record struct {
	Type       string              `json:"type"`
	ID         string              `json:"source_component_id"`
	Name       string              `json:"name"`
	FType      string              `json:"ftype"`
	DoNotPlace bool                `json:"do_not_place"`
	Supplier   map[string][]string `json:"supplier_part_numbers"`
}

type catalog struct {
	ID           string         `json:"lcsc_id"`
	Name         string         `json:"name"`
	Package      string         `json:"package"`
	Manufacturer string         `json:"manufacturer"`
	Library      string         `json:"library_type"`
	Stock        *float64       `json:"stock"`
	Price        *float64       `json:"price"`
	Attributes   map[string]any `json:"attributes"`
	Datasheet    string         `json:"datasheet"`
}

type Entry struct {
	Key            string         `json:"key"`
	Value          string         `json:"value"`
	Package        string         `json:"package"`
	LCSC           string         `json:"lcsc"`
	MPN            string         `json:"mpn"`
	Manufacturer   string         `json:"mfr,omitempty"`
	JLCType        string         `json:"jlc_type"`
	Stock          float64        `json:"stock"`
	Price          float64        `json:"price_usd"`
	Verified       bool           `json:"verified"`
	VerifiedOn     *string        `json:"verified_on"`
	CatalogPackage string         `json:"catalog_package"`
	Attributes     map[string]any `json:"attributes"`
	Datasheet      string         `json:"datasheet,omitempty"`
	URL            string         `json:"url"`
	UsedBy         []string       `json:"used_by"`
}

var validID = regexp.MustCompile(`^C[0-9]+$`)
var passive = regexp.MustCompile(`\b(0402|0603|0805|1206|1210|2010|2512)\b`)

func packageName(name string) string {
	if match := passive.FindString(name); match != "" {
		return match
	}
	return name
}

// Ingest atomically replaces partsPath only after every placed part validates.
// Offline ingestion never runs a subprocess or refreshes a verification date.
func Ingest(ctx context.Context, circuitPath, partsPath, catalogDir string, offline bool) ([]Entry, error) {
	data, err := os.ReadFile(circuitPath)
	if err != nil {
		return nil, err
	}
	var records []record
	if err = json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	var previous []Entry
	if data, err = os.ReadFile(partsPath); err == nil {
		if err = json.Unmarshal(data, &previous); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	old := map[string]Entry{}
	for _, p := range previous {
		old[p.LCSC] = p
	}
	placed := map[string]bool{}
	for _, r := range records {
		if r.Type == "pcb_component" {
			placed[r.ID] = !r.DoNotPlace
		}
		for _, id := range r.Supplier["jlcpcb"] {
			if !validID.MatchString(id) {
				return nil, fmt.Errorf("invalid LCSC id %q", id)
			}
		}
	}
	used := map[string][]string{}
	for _, r := range records {
		if r.Type != "source_component" || !placed[r.ID] || r.FType == "simple_test_point" {
			continue
		}
		ids := r.Supplier["jlcpcb"]
		if len(ids) == 0 {
			return nil, fmt.Errorf("%s: no LCSC number", r.Name)
		}
		used[ids[0]] = append(used[ids[0]], r.Name)
	}
	ids := make([]string, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if err = os.MkdirAll(catalogDir, 0755); err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(ids))
	for _, id := range ids {
		file := filepath.Join(catalogDir, id+".json")
		var raw json.RawMessage
		if offline {
			raw, err = os.ReadFile(file)
			if err != nil {
				return nil, fmt.Errorf("%s: missing offline catalog response: %w", id, err)
			}
		} else {
			raw, err = lookup(ctx, id)
			if err != nil {
				return nil, err
			}
		}
		var c catalog
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, err
		}
		if c.ID != id {
			return nil, fmt.Errorf("%s: cached catalog ID mismatch (%s)", id, c.ID)
		}
		if (c.Library != "basic" && c.Library != "extended") || c.Name == "" || c.Package == "" ||
			c.Stock == nil || *c.Stock < 0 || c.Price == nil || *c.Price < 0 {
			return nil, fmt.Errorf("%s: incomplete catalog classification/value/package/stock/price", id)
		}
		if !offline {
			if err = cache.WriteJSON(file, raw); err != nil {
				return nil, err
			}
		}
		p := old[id]
		verified, date := false, p.VerifiedOn
		if !offline {
			today := time.Now().UTC().Format("2006-01-02")
			date, verified = &today, true
		} else if date != nil && *date != "" {
			_, err := time.Parse("2006-01-02", *date)
			verified = p.Verified && err == nil
		}
		datasheet := c.Datasheet
		if datasheet == "" {
			datasheet = p.Datasheet
		}
		kind := "Extended"
		if c.Library == "basic" {
			kind = "Basic"
		}
		attrs := c.Attributes
		if attrs == nil {
			attrs = map[string]any{}
		}
		entries = append(entries, Entry{
			Key: id, Value: c.Name, Package: packageName(c.Package), LCSC: id, MPN: c.Name,
			Manufacturer: c.Manufacturer, JLCType: kind, Stock: *c.Stock, Price: *c.Price,
			Verified: verified, VerifiedOn: date, CatalogPackage: packageName(c.Package),
			Attributes: attrs, Datasheet: datasheet, URL: "https://jlcpcb.com/partdetail/" + id, UsedBy: used[id],
		})
	}
	if err = cache.WriteJSON(partsPath, entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func lookup(ctx context.Context, id string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npx", "@jlcpcb/cli", "search", id, "--json", "--limit", "5")
	cmd.Stderr = os.Stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: catalog search: %w", id, err)
	}
	var items []json.RawMessage
	if err = json.Unmarshal(data, &items); err != nil {
		var object map[string]json.RawMessage
		if err = json.Unmarshal(data, &object); err != nil {
			return nil, err
		}
		// Inspect every array in a response envelope, independent of map order.
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			var list []json.RawMessage
			if json.Unmarshal(object[key], &list) == nil {
				items = append(items, list...)
			}
		}
	}
	for _, raw := range items {
		var c catalog
		if json.Unmarshal(raw, &c) == nil && c.ID == id {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("%s: not found in JLC/LCSC catalog", id)
}
