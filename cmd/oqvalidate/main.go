package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/validate"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "verify-shared" {
		path, e := validate.VerifyShared(os.Args[2])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		fmt.Println(path)
		return
	}
	preflight := flag.String("preflight-cache", "", "verified oqcache outputs from the same source check; requires -shared-spec")
	spec := flag.String("shared-spec", "", "dependency specification: run source/schematic checks once for the batch")
	jobs := flag.Int("jobs", 2, "concurrent validators")
	out := flag.String("out", "", "new output directory")
	script := flag.String("script", "", "required: candidate-capable Bash package script")
	flag.Parse()
	if *script == "" || (*preflight != "" && *spec == "") || *out == "" || flag.NArg() == 0 || *jobs < 1 {
		fmt.Fprintln(os.Stderr, "usage: oqvalidate -script package.sh -out new-directory [-jobs 2] candidate.kicad_pcb... (in ranked order)")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	abs, err := filepath.Abs(*out)
	if err != nil {
		panic(err)
	}
	inputs := flag.Args()
	for i := range inputs {
		inputs[i], err = filepath.Abs(inputs[i])
		if err != nil {
			panic(err)
		}
	}
	shared := ""
	if *spec != "" {
		if *preflight != "" {
			shared, err = validate.AdoptCached(*script, *spec, *preflight, abs+"-source")
		} else {
			shared, err = validate.PrepareShared(ctx, *script, *spec, abs+"-source")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("shared source checks:", shared)
	}
	results, err := validate.RunWithShared(ctx, *script, abs, inputs, *jobs, shared)
	for _, r := range results {
		fmt.Printf("candidate validation: %s passed=%t %.2fs %s\n", r.Input, r.Passed, r.Seconds, r.Output)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, r := range results {
		if r.Passed {
			fmt.Println("selected candidate:", r.Input)
			break
		}
	}
}
