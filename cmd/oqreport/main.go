// oqreport prints a package log's release-gate lines and verifies a release manifest, so status docs are
// generated from measured results instead of transcribed.
//
//	oqreport gates <package.log>
//	oqreport manifest <release-manifest.txt> [repo-root]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/benarent/pcb-tools/internal/report"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: oqreport gates <package.log> | oqreport manifest <manifest.txt> [repo-root]")
		os.Exit(2)
	}
	command := os.Args[1]
	args := os.Args[2:]
	if command == "gates" || command == "validation" {
		flags := flag.NewFlagSet(command, flag.ExitOnError)
		prefixes := flags.String("prefixes", "", "JSON file of gate-line prefixes; replaces the compatibility defaults")
		flags.Parse(args)
		args = flags.Args()
		if *prefixes != "" {
			data, err := os.ReadFile(*prefixes)
			if err != nil {
				fail(err)
			}
			if err = json.Unmarshal(data, &report.GatePrefixes); err != nil {
				fail(err)
			}
			if len(report.GatePrefixes) == 0 {
				fail(fmt.Errorf("at least one gate prefix required"))
			}
			for _, prefix := range report.GatePrefixes {
				if strings.TrimSpace(prefix) == "" {
					fail(fmt.Errorf("empty gate prefix"))
				}
			}
		}
	}
	// Keep the positional command handlers consistent after flag parsing.
	os.Args = append([]string{os.Args[0], command}, args...)
	switch command {
	case "validation":
		if len(os.Args) != 6 {
			fail(fmt.Errorf("usage: oqreport validation log manifest document root"))
		}
		section, err := report.ValidationSection(os.Args[2], os.Args[3], os.Args[5])
		if err != nil {
			fail(err)
		}
		doc, err := os.ReadFile(os.Args[4])
		if err != nil {
			fail(err)
		}
		next, err := report.UpdateSection(string(doc), section)
		if err != nil {
			fail(err)
		}
		if err = os.WriteFile(os.Args[4], []byte(next), 0644); err != nil {
			fail(err)
		}
		fmt.Println("generated validation evidence:", os.Args[4])
	case "gates":
		if len(args) != 1 {
			fail(fmt.Errorf("usage: oqreport gates [-prefixes prefixes.json] log"))
		}
		f, err := os.Open(os.Args[2])
		if err != nil {
			fail(err)
		}
		defer f.Close()
		lines, err := report.Gates(f)
		if err != nil {
			fail(err)
		}
		for _, l := range lines {
			fmt.Println(l)
		}
	case "manifest":
		if len(args) < 1 || len(args) > 2 {
			fail(fmt.Errorf("usage: oqreport manifest manifest [root]"))
		}
		root := "."
		if len(os.Args) > 3 {
			root = os.Args[3]
		}
		ok, total, problems, err := report.VerifyManifest(os.Args[2], root)
		if err != nil {
			fail(err)
		}
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "  ✗", p)
		}
		fmt.Printf("manifest hashes verified: %d/%d\n", ok, total)
		if ok != total || total == 0 {
			os.Exit(1)
		}
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "oqreport:", err)
	os.Exit(1)
}
