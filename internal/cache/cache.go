// Package cache fingerprints explicit dependency trees and caches successful commands.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type Spec struct {
	Paths  []string          `json:"paths"`
	Values map[string]string `json:"values"`
}

// Key includes file names, contents, modes, symlink targets (and their contents),
// tool identities and configuration. Missing dependencies fail closed.
func Key(s Spec) (string, error) {
	h := sha256.New()
	values, err := json.Marshal(s.Values)
	if err != nil {
		return "", err
	}
	h.Write(values)
	paths := append([]string(nil), s.Paths...)
	sort.Strings(paths)
	var visit func(string, string, map[string]bool) error
	visit = func(path, name string, seen map[string]bool) error {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%q %v\n", name, info.Mode())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			target, err = filepath.Abs(target)
			if err != nil {
				return err
			}
			if seen[target] {
				return fmt.Errorf("symlink cycle: %s", path)
			}
			seen[target] = true
			defer delete(seen, target)
			fmt.Fprintf(h, "target %q\n", target)
			return visit(target, name, seen)
		}
		if info.IsDir() {
			entries, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			for _, e := range entries {
				// Ignore OS metadata and Python bytecode; enumerate source trees explicitly.
				if e.Name() == "__pycache__" || e.Name() == ".DS_Store" {
					continue
				}
				if err := visit(filepath.Join(path, e.Name()), name+"/"+e.Name(), seen); err != nil {
					return err
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported input: %s", path)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		fmt.Fprintln(h)
		return err
	}
	for _, p := range paths {
		if err := visit(p, p, map[string]bool{}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func WriteJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".oq-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
