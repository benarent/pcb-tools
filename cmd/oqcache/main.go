// oqcache run SPEC CACHE_DIR -- COMMAND... caches only a successful preflight.
// {cache} in a command argument expands to a new isolated result directory.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"strings"
)

type receipt struct {
	Key     string
	Outputs string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "oqcache:", err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 3 && os.Args[1] == "key" {
		b, err := os.ReadFile(os.Args[2])
		if err != nil {
			return err
		}
		var s cache.Spec
		if err = json.Unmarshal(b, &s); err != nil {
			return err
		}
		if len(s.Paths) == 0 {
			return fmt.Errorf("empty dependency list")
		}
		key, err := cache.Key(s)
		if err != nil {
			return err
		}
		fmt.Println(key)
		return nil
	}
	if len(os.Args) < 6 || os.Args[1] != "run" || os.Args[4] != "--" {
		return fmt.Errorf("usage: oqcache run spec.json cache-dir -- command [args; {cache} expands to output directory]")
	}
	b, err := os.ReadFile(os.Args[2])
	if err != nil {
		return err
	}
	var spec cache.Spec
	if err = json.Unmarshal(b, &spec); err != nil {
		return err
	}
	if len(spec.Paths) == 0 {
		return fmt.Errorf("empty dependency list")
	}
	// The command and its literal arguments are dependencies too.
	if spec.Values == nil {
		spec.Values = map[string]string{}
	}
	spec.Values["command"] = strings.Join(os.Args[5:], "\x00")
	key, err := cache.Key(spec)
	if err != nil {
		return err
	}
	base, err := filepath.Abs(os.Args[3])
	if err != nil {
		return err
	}
	dir := filepath.Join(base, key)
	var r receipt
	if b, err := os.ReadFile(filepath.Join(dir, "receipt.json")); err == nil && json.Unmarshal(b, &r) == nil && r.Key == key {
		got, err := cache.Key(cache.Spec{Paths: []string{filepath.Join(dir, "outputs")}})
		if err == nil && got == r.Outputs {
			fmt.Fprintln(os.Stderr, "preflight cache: HIT", key)
			fmt.Println(filepath.Join(dir, "outputs"))
			return nil
		}
	}
	fmt.Fprintln(os.Stderr, "preflight cache: MISS", key)
	if err = os.MkdirAll(base, 0755); err != nil {
		return err
	}
	// Never let simultaneous writers replace an entry while another caller reads it.
	lock := filepath.Join(base, key+".lock")
	if err = os.Mkdir(lock, 0755); err != nil {
		return fmt.Errorf("cache key is busy (remove stale lock only after checking its owner): %w", err)
	}
	defer os.Remove(lock)
	tmp, err := os.MkdirTemp(base, ".pending-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "outputs")
	args := append([]string(nil), os.Args[5:]...)
	for i := range args {
		args[i] = strings.ReplaceAll(args[i], "{cache}", out)
	}
	cmd := exec.Command(args[0], args[1:]...)
	log, err := os.Create(filepath.Join(tmp, "preflight.log"))
	if err != nil {
		return err
	}
	cmd.Stdout = log
	cmd.Stderr = log
	err = cmd.Run()
	log.Close()
	if err != nil {
		b, _ := os.ReadFile(filepath.Join(tmp, "preflight.log"))
		os.Stderr.Write(b)
		return err
	}
	after, err := cache.Key(spec)
	if err != nil {
		return err
	}
	if after != key {
		return fmt.Errorf("dependencies changed during preflight; result not cached")
	}
	// Publish outputs before hashing because absolute names are part of the fingerprint.
	if err = os.RemoveAll(dir); err != nil {
		return err
	}
	if err = os.Rename(tmp, dir); err != nil {
		return err
	}
	outputs, err := cache.Key(cache.Spec{Paths: []string{filepath.Join(dir, "outputs")}})
	if err != nil {
		return err
	}
	if err = cache.WriteJSON(filepath.Join(dir, "receipt.json"), receipt{key, outputs}); err != nil {
		return err
	}
	fmt.Println(filepath.Join(dir, "outputs"))
	return nil
}
