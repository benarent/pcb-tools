package pcbapps

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"regexp"
	"strings"
	"time"
)

type ReleasePlan struct {
	Schema         int      `json:"schema"`
	Paths          []string `json:"snapshot_paths"`
	Command        []string `json:"command"`
	Manifest       string   `json:"manifest"`
	PCB            string   `json:"pcb"`
	Review         string   `json:"review"`
	RequiredChecks []string `json:"required_checks"`
	Extras         []string `json:"extras"`
}
type ReleaseReceipt struct {
	Schema   int               `json:"schema"`
	Created  string            `json:"created_utc"`
	InputKey string            `json:"input_key"`
	PCB      string            `json:"pcb_sha256"`
	Command  []string          `json:"command"`
	Checks   []string          `json:"review_checks"`
	Hashes   map[string]string `json:"sha256"`
}

func Release(args []string) error {
	if len(args) == 2 && args[0] == "verify" {
		return VerifyRelease(args[1])
	}
	if len(args) != 3 || args[0] != "build" {
		return fmt.Errorf("usage: oqrelease build plan.json new-bundle-directory | oqrelease verify bundle-directory")
	}
	var p ReleasePlan
	if e := ReadJSON(args[1], &p); e != nil {
		return e
	}
	root, e := os.Getwd()
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return BuildRelease(ctx, root, p, args[2])
}
func validatePlan(p ReleasePlan) error {
	if p.Schema != 1 || len(p.Paths) == 0 || len(p.Command) == 0 || len(p.RequiredChecks) == 0 {
		return fmt.Errorf("nonempty schema 1 snapshot paths, command and required checks needed")
	}
	for _, s := range append(append(append([]string{}, p.Paths...), p.Extras...), p.Manifest, p.PCB, p.Review) {
		if !SafeRelative(s) {
			return fmt.Errorf("unsafe relative release path %q", s)
		}
	}
	return nil
}
func BuildRelease(ctx context.Context, root string, p ReleasePlan, target string) error {
	if e := validatePlan(p); e != nil {
		return e
	}
	target, e := filepath.Abs(target)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(target); !os.IsNotExist(e) {
		return fmt.Errorf("bundle target already exists or cannot be inspected: %s", target)
	}
	if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
		return e
	}
	lock := target + ".lock"
	if e = os.Mkdir(lock, 0700); e != nil {
		return fmt.Errorf("release target locked: %w", e)
	}
	defer os.Remove(lock)
	stage, e := os.MkdirTemp(filepath.Dir(target), ".oqrelease-")
	if e != nil {
		return e
	}
	published := false
	defer func() {
		if published {
			os.RemoveAll(stage)
		} else {
			failed := target + ".failed"
			if e := os.Rename(stage, failed); e != nil {
				fmt.Fprintln(os.Stderr, "failed workspace retained:", stage)
			} else {
				fmt.Fprintln(os.Stderr, "failed workspace retained:", failed)
			}
		}
	}()
	// Failure evidence lives outside the unpublished staging directory.
	log, e := os.OpenFile(target+".build.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer log.Close()
	spec := cache.Spec{Values: map[string]string{"plan": mustJSON(p)}}
	for _, v := range p.Paths {
		src := filepath.Join(root, v)
		rel, e := filepath.Rel(src, target)
		if e != nil {
			return e
		}
		if rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("target must be outside snapshot inputs")
		}
		spec.Paths = append(spec.Paths, src)
	}
	before, e := cache.Key(spec)
	if e != nil {
		return e
	}
	workspace := filepath.Join(stage, "workspace")
	if e = os.Mkdir(workspace, 0755); e != nil {
		return e
	}
	for _, v := range p.Paths {
		if e = copyTree(filepath.Join(root, v), filepath.Join(workspace, v), root, false); e != nil {
			return e
		}
	}
	after, e := cache.Key(spec)
	if e != nil {
		return e
	}
	if before != after {
		return fmt.Errorf("inputs changed while snapshotting")
	}
	cmd := exec.CommandContext(ctx, p.Command[0], p.Command[1:]...)
	cmd.Dir = workspace
	cmd.Stdout = io.MultiWriter(os.Stdout, log)
	cmd.Stderr = io.MultiWriter(os.Stderr, log)
	cmd.Env = releaseEnv(os.Environ())
	cmd.Env = append(cmd.Env, "OQ_RELEASE_STAGE=1")
	if e = cmd.Run(); e != nil {
		return fmt.Errorf("package failed; previous bundles unchanged; log %s: %w", target+".build.log", e)
	}
	if e = log.Sync(); e != nil {
		return e
	}
	bundle := filepath.Join(stage, "bundle")
	receipt, e := assembleRelease(workspace, bundle, p, before)
	if e != nil {
		return e
	}
	if e = copyTree(target+".build.log", filepath.Join(bundle, "build.log"), filepath.Dir(target), true); e != nil {
		return e
	}
	receipt.Hashes["build.log"], e = Hash(filepath.Join(bundle, "build.log"))
	if e != nil {
		return e
	}
	if e = Write(filepath.Join(bundle, "release.json"), receipt); e != nil {
		return e
	}
	if e = VerifyRelease(bundle); e != nil {
		return e
	}
	// Rename a complete verified directory on the same filesystem. Never replace a prior release.
	if _, e = os.Lstat(target); !os.IsNotExist(e) {
		return fmt.Errorf("target appeared during build")
	}
	if e = os.Rename(bundle, target); e != nil {
		return e
	}
	published = true
	fmt.Printf("release published: %s (%d verified files; PCB %s)\n", target, len(receipt.Hashes), receipt.PCB)
	return nil
}
func releaseEnv(env []string) []string {
	var out []string
	for _, v := range env {
		key := strings.SplitN(v, "=", 2)[0]
		if key == "OQ_RELEASE_STAGE" || key == "OQ_SHARED_CHECK" || key == "OQ_CHECK_OUTPUT" {
			continue
		}
		out = append(out, v)
	}
	return out
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func copyTree(src, dst, root string, regularOnly bool) error {
	if regularOnly {
		rel, e := filepath.Rel(root, src)
		if e != nil || !SafeRelative(rel) {
			return fmt.Errorf("artifact escapes root")
		}
		p := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			p = filepath.Join(p, part)
			i, e := os.Lstat(p)
			if e != nil {
				return e
			}
			if i.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in artifact path %s", p)
			}
		}
	}
	info, e := os.Lstat(src)
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if regularOnly {
			return fmt.Errorf("symlink forbidden in release artifacts: %s", src)
		}
		link, e := os.Readlink(src)
		if e != nil {
			return e
		}
		resolved, e := filepath.EvalSymlinks(src)
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, resolved)
		if e != nil || !SafeRelative(rel) || filepath.IsAbs(link) {
			return fmt.Errorf("external/absolute input symlink forbidden: %s", src)
		}
		if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
			return e
		}
		return os.Symlink(link, dst)
	}
	if info.IsDir() {
		if e = os.MkdirAll(dst, info.Mode().Perm()); e != nil {
			return e
		}
		entries, e := os.ReadDir(src)
		if e != nil {
			return e
		}
		for _, v := range entries {
			if v.Name() == ".DS_Store" || v.Name() == "__pycache__" {
				continue
			}
			if e = copyTree(filepath.Join(src, v.Name()), filepath.Join(dst, v.Name()), root, regularOnly); e != nil {
				return e
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file %s", src)
	}
	if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
		return e
	}
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}
	return ce
}

var shaLine = regexp.MustCompile(`^([0-9a-f]{64})  (.+)$`)

func manifestEntries(path string) (map[string]string, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	m := map[string]string{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1<<20)
	started := false
	for s.Scan() {
		line := s.Text()
		if line == "SHA-256" {
			started = true
			continue
		}
		match := shaLine.FindStringSubmatch(line)
		if match == nil {
			if started && strings.TrimSpace(line) != "" {
				return nil, fmt.Errorf("malformed manifest hash entry")
			}
			continue
		}
		started = true
		p := match[2]
		if !SafeRelative(p) || m[p] != "" {
			return nil, fmt.Errorf("unsafe/duplicate manifest path %s", p)
		}
		m[p] = match[1]
	}
	if e = s.Err(); e != nil {
		return nil, e
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("empty manifest")
	}
	return m, nil
}
func assembleRelease(root, bundle string, p ReleasePlan, key string) (ReleaseReceipt, error) {
	r := ReleaseReceipt{Schema: 1, Created: time.Now().UTC().Format(time.RFC3339), InputKey: key, Command: p.Command, Checks: p.RequiredChecks, Hashes: map[string]string{}}
	entries, e := manifestEntries(filepath.Join(root, p.Manifest))
	if e != nil {
		return r, e
	}
	for _, required := range []string{p.PCB, p.Review} {
		if entries[required] == "" {
			return r, fmt.Errorf("manifest missing required %s", required)
		}
	}
	var review struct {
		ArtifactHashes map[string]string `json:"artifact_sha256"`
		PCB            string            `json:"pcb_sha256"`
		Checks         map[string]struct {
			Status   string `json:"status"`
			Evidence string `json:"evidence"`
		} `json:"checks"`
	}
	data, e := os.ReadFile(filepath.Join(root, p.Review))
	if e != nil {
		return r, e
	}
	if e = json.Unmarshal(data, &review); e != nil {
		return r, e
	}
	if review.PCB != entries[p.PCB] {
		return r, fmt.Errorf("review PCB hash mismatch")
	}
	for _, name := range p.RequiredChecks {
		v, ok := review.Checks[name]
		if !ok || v.Status != "verified" || strings.TrimSpace(v.Evidence) == "" {
			return r, fmt.Errorf("unverified release review %s", name)
		}
	}
	for name, v := range review.Checks {
		if v.Status != "verified" || strings.TrimSpace(v.Evidence) == "" {
			return r, fmt.Errorf("unverified extra release review %s", name)
		}
	}
	for path, want := range review.ArtifactHashes {
		if !SafeRelative(path) || !hashValid(want) {
			return r, fmt.Errorf("invalid reviewed artifact %s", path)
		}
		got, e := Hash(filepath.Join(root, path))
		if e != nil {
			return r, e
		}
		if got != want {
			return r, fmt.Errorf("reviewed artifact changed: %s", path)
		}
	}
	r.PCB = review.PCB
	if e = os.MkdirAll(bundle, 0755); e != nil {
		return r, e
	}
	for _, path := range Keys(entries) {
		src := filepath.Join(root, path)
		if e = copyTree(src, filepath.Join(bundle, path), root, true); e != nil {
			return r, e
		}
		h, e := Hash(filepath.Join(bundle, path))
		if e != nil {
			return r, e
		}
		if h != entries[path] {
			return r, fmt.Errorf("manifest mismatch %s", path)
		}
		r.Hashes[path] = h
	}
	for _, path := range append([]string{p.Manifest}, p.Extras...) {
		if r.Hashes[path] != "" {
			continue
		}
		if e = copyTree(filepath.Join(root, path), filepath.Join(bundle, path), root, true); e != nil {
			return r, e
		}
	}
	e = filepath.WalkDir(bundle, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(bundle, path)
		if e != nil {
			return e
		}
		h, e := Hash(path)
		r.Hashes[rel] = h
		return e
	})
	if e == nil {
		for path, want := range review.ArtifactHashes {
			if r.Hashes[path] != want {
				return r, fmt.Errorf("reviewed artifact omitted/changed in bundle: %s", path)
			}
		}
	}
	return r, e
}
func VerifyRelease(dir string) error {
	var r ReleaseReceipt
	if e := ReadJSON(filepath.Join(dir, "release.json"), &r); e != nil {
		return e
	}
	if r.Schema != 1 || len(r.Hashes) == 0 || len(r.PCB) != 64 || len(r.Checks) == 0 {
		return fmt.Errorf("invalid release receipt")
	}
	for _, p := range Keys(r.Hashes) {
		if !SafeRelative(p) {
			return fmt.Errorf("unsafe receipt path")
		}
		i, e := os.Lstat(filepath.Join(dir, p))
		if e != nil {
			return e
		}
		if !i.Mode().IsRegular() {
			return fmt.Errorf("nonregular artifact %s", p)
		}
		h, e := Hash(filepath.Join(dir, p))
		if e != nil {
			return e
		}
		if h != r.Hashes[p] {
			return fmt.Errorf("release artifact changed: %s", p)
		}
	}
	e := filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		if rel != "release.json" && r.Hashes[rel] == "" {
			return fmt.Errorf("unlisted release artifact %s", rel)
		}
		return nil
	})
	if e == nil {
		fmt.Printf("release verification: %d/%d files\n", len(r.Hashes), len(r.Hashes))
	}
	return e
}

// FileDigest is also used by tests without exposing receipt implementation details.
func FileDigest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
