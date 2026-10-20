// Package report extracts release-gate lines from a package log and verifies a SHA-256 release manifest.
package report

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GatePrefixes are the summary lines each OakQuake package stage prints, in pipeline order.
var GatePrefixes = []string{
	"circuit health", "catalog value/package audit", "power corner calculations", "electrical invariants",
	"electrical gate fault tests", "pre-route model fit", "USB mechanical", "SD access", "Duplicate segments",
	"Single-layer", "3D illustration", "v2.4 footprint regression", "netlist check", "Found ", "schematic netlist check",
	"DRC:", "DC copper", "SI geometry", "CPL reference fit", "BOM:", "JLCPCB classification", "assembly check",
	"v2.4 engineering release review", "fabrication archive check",
	"routing integrity:", "Go route transfer:", "footprint contracts:", "mechanical envelopes:",
	"pre-route silkscreen:", "ground reservations:", "long-net report:",
}

// ValidationSection only presents gate output after a nonempty manifest verifies.
func ValidationSection(log, manifest, root string) (string, error) {
	ok, total, problems, err := VerifyManifest(manifest, root)
	if err != nil {
		return "", err
	}
	if total == 0 || ok != total {
		return "", fmt.Errorf("manifest %d/%d: %v", ok, total, problems)
	}
	f, err := os.Open(log)
	if err != nil {
		return "", err
	}
	defer f.Close()
	lines, err := Gates(f)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("no gate lines in log")
	}
	return fmt.Sprintf("<!-- oqreport:start -->\n## Generated validation evidence\n\nSource log: `%s`. Manifest: **%d/%d hashes verified**.\n\nVerbatim gate lines (CAD checks and stated screening models; not bench qualification):\n\n```text\n%s\n```\n<!-- oqreport:end -->", filepath.Base(log), ok, total, strings.Join(lines, "\n")), nil
}

func UpdateSection(document, section string) (string, error) {
	const start = "<!-- oqreport:start -->"
	const end = "<!-- oqreport:end -->"
	a, b := strings.Index(document, start), strings.Index(document, end)
	if a < 0 && b < 0 {
		return strings.TrimRight(document, "\n") + "\n\n" + section + "\n", nil
	}
	if a < 0 || b < a || strings.Count(document, start) != 1 || strings.Count(document, end) != 1 {
		return "", fmt.Errorf("invalid oqreport markers")
	}
	return document[:a] + section + document[b+len(end):], nil
}

// Gates returns the gate lines found in a package log, in log order.
func Gates(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := sc.Text()
		for _, p := range GatePrefixes {
			if strings.HasPrefix(line, p) {
				out = append(out, line)
				break
			}
		}
	}
	return out, sc.Err()
}

var manifestLine = regexp.MustCompile(`^([0-9a-f]{64})  (.+)$`)

// VerifyManifest checks every SHA-256 entry (paths relative to root) and returns verified and total counts
// plus a description of each mismatch or missing file.
func VerifyManifest(manifest, root string) (ok, total int, problems []string, err error) {
	f, err := os.Open(manifest)
	if err != nil {
		return 0, 0, nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := manifestLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		total++
		path := m[2]
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		got, herr := hashFile(path)
		switch {
		case herr != nil:
			problems = append(problems, fmt.Sprintf("%s: %v", m[2], herr))
		case got != m[1]:
			problems = append(problems, fmt.Sprintf("%s: hash changed", m[2]))
		default:
			ok++
		}
	}
	return ok, total, problems, sc.Err()
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
