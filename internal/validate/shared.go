package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"strings"
)

type SharedReceipt struct {
	Schema      int        `json:"schema"`
	Spec        cache.Spec `json:"dependencies"`
	Key         string     `json:"dependency_key"`
	Artifacts   string     `json:"artifacts"`
	ArtifactKey string     `json:"artifact_key"`
}

func PrepareShared(ctx context.Context, script, specPath, dir string) (string, error) {
	data, e := os.ReadFile(specPath)
	if e != nil {
		return "", e
	}
	var spec cache.Spec
	if e = json.Unmarshal(data, &spec); e != nil {
		return "", e
	}
	if len(spec.Paths) == 0 {
		return "", fmt.Errorf("empty shared dependency inventory")
	}
	if spec.Values == nil {
		spec.Values = map[string]string{}
	}
	spec.Values["shared_command"] = "bash\x00" + script + "\x00check"
	key, e := cache.Key(spec)
	if e != nil {
		return "", e
	}
	if e = os.Mkdir(dir, 0755); e != nil {
		return "", e
	}
	artifact := filepath.Join(dir, "artifacts")
	log, e := os.Create(filepath.Join(dir, "source.log"))
	if e != nil {
		return "", e
	}
	defer log.Close()
	cmd := exec.CommandContext(ctx, "bash", script, "check")
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Env = sharedEnv(os.Environ())
	cmd.Env = append(cmd.Env, "OQ_CHECK_OUTPUT="+artifact)
	if e = cmd.Run(); e != nil {
		return "", fmt.Errorf("shared checks failed (see %s): %w", log.Name(), e)
	}
	after, e := cache.Key(spec)
	if e != nil {
		return "", e
	}
	if key != after {
		return "", fmt.Errorf("dependencies changed during shared checks")
	}
	outkey, e := cache.Key(cache.Spec{Paths: []string{artifact}})
	if e != nil {
		return "", e
	}
	path := filepath.Join(dir, "receipt.json")
	if e = cache.WriteJSON(path, SharedReceipt{1, spec, key, artifact, outkey}); e != nil {
		return "", e
	}
	return path, nil
}
func VerifyShared(path string) (string, error) {
	data, e := os.ReadFile(path)
	if e != nil {
		return "", e
	}
	var r SharedReceipt
	if e = json.Unmarshal(data, &r); e != nil {
		return "", e
	}
	if r.Schema != 1 || len(r.Spec.Paths) == 0 || !filepath.IsAbs(r.Artifacts) {
		return "", fmt.Errorf("invalid shared-check receipt")
	}
	key, e := cache.Key(r.Spec)
	if e != nil {
		return "", e
	}
	if key != r.Key {
		return "", fmt.Errorf("shared-check dependencies changed")
	}
	key, e = cache.Key(cache.Spec{Paths: []string{r.Artifacts}})
	if e != nil {
		return "", e
	}
	if key != r.ArtifactKey {
		return "", fmt.Errorf("shared-check artifacts changed")
	}
	return r.Artifacts, nil
}
func sharedEnv(env []string) []string {
	var out []string
	for _, v := range env {
		key := strings.SplitN(v, "=", 2)[0]
		if key == "OQ_SHARED_CHECK" || key == "OQ_CHECK_OUTPUT" {
			continue
		}
		out = append(out, v)
	}
	return out
}

// AdoptCached reuses oqcache's already successful source check. It verifies the
// exact command, dependency key and output digest before creating a batch receipt.
func AdoptCached(script, specPath, artifacts, dir string) (string, error) {
	data, e := os.ReadFile(specPath)
	if e != nil {
		return "", e
	}
	var spec cache.Spec
	if e = json.Unmarshal(data, &spec); e != nil {
		return "", e
	}
	if len(spec.Paths) == 0 {
		return "", fmt.Errorf("empty dependency inventory")
	}
	if spec.Values == nil {
		spec.Values = map[string]string{}
	}
	spec.Values["command"] = strings.Join([]string{"env", "OQ_CHECK_OUTPUT={cache}", "bash", script, "check"}, "\x00")
	key, e := cache.Key(spec)
	if e != nil {
		return "", e
	}
	artifacts, e = filepath.Abs(artifacts)
	if e != nil {
		return "", e
	}
	if filepath.Base(artifacts) != "outputs" {
		return "", fmt.Errorf("expected oqcache outputs directory")
	}
	var cached struct{ Key, Outputs string }
	data, e = os.ReadFile(filepath.Join(filepath.Dir(artifacts), "receipt.json"))
	if e != nil {
		return "", e
	}
	if e = json.Unmarshal(data, &cached); e != nil {
		return "", e
	}
	if cached.Key != key {
		return "", fmt.Errorf("cached source command/dependencies differ")
	}
	outputKey, e := cache.Key(cache.Spec{Paths: []string{artifacts}})
	if e != nil {
		return "", e
	}
	if outputKey != cached.Outputs {
		return "", fmt.Errorf("cached source artifacts changed")
	}
	if e = os.Mkdir(dir, 0755); e != nil {
		return "", e
	}
	path := filepath.Join(dir, "receipt.json")
	if e = cache.WriteJSON(path, SharedReceipt{1, spec, key, artifacts, outputKey}); e != nil {
		return "", e
	}
	return path, nil
}
