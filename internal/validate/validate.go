// Package validate validates ranked candidates concurrently without sharing output paths.
package validate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"github.com/benarent/pcb-tools/internal/cache"
	"sync"
	"time"
)

type Result struct {
	Input    string  `json:"input"`
	Snapshot string  `json:"snapshot"`
	SHA256   string  `json:"input_sha256"`
	Output   string  `json:"output"`
	Passed   bool    `json:"passed"`
	Error    string  `json:"error,omitempty"`
	Shared   string  `json:"shared_receipt,omitempty"`
	Seconds  float64 `json:"seconds"`
}

func Run(ctx context.Context, script, output string, inputs []string, jobs int) ([]Result, error) {
	return RunWithShared(ctx, script, output, inputs, jobs, "")
}
func RunWithShared(ctx context.Context, script, output string, inputs []string, jobs int, shared string) ([]Result, error) {
	if shared != "" {
		if _, err := VerifyShared(shared); err != nil {
			return nil, err
		}
	}
	if jobs < 1 || len(inputs) == 0 {
		return nil, fmt.Errorf("positive jobs and at least one candidate required")
	}
	if err := os.Mkdir(output, 0755); err != nil {
		return nil, fmt.Errorf("output must be a new directory: %w", err)
	}
	results := make([]Result, len(inputs))
	queue := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				start := time.Now()
				r := Result{Shared: shared, Input: inputs[i], Output: filepath.Join(output, fmt.Sprintf("c%d", i+1))}
				b, err := os.ReadFile(r.Input)
				if err == nil {
					r.SHA256 = fmt.Sprintf("%x", sha256.Sum256(b))
					// Copy input before the child starts so edits to the original cannot race validation.
					snapshot := filepath.Join(output, fmt.Sprintf("input-%d.kicad_pcb", i+1))
					r.Snapshot = snapshot
					err = os.WriteFile(snapshot, b, 0644)
					if err == nil {
						log, e := os.Create(filepath.Join(output, fmt.Sprintf("c%d.log", i+1)))
						err = e
						if err == nil {
							cmd := exec.CommandContext(ctx, "bash", script, "candidate", snapshot, r.Output)
							cmd.Env = sharedEnv(os.Environ())
							if shared != "" {
								cmd.Env = append(cmd.Env, "OQ_SHARED_CHECK="+shared)
							}
							cmd.Stdout = log
							cmd.Stderr = log
							err = cmd.Run()
							log.Close()
						}
					}
				}
				if err == nil && shared != "" {
					_, err = VerifyShared(shared)
				}
				r.Passed = err == nil
				if err != nil {
					r.Error = err.Error()
				}
				r.Seconds = time.Since(start).Seconds()
				results[i] = r
			}
		}()
	}
	for i := range inputs {
		queue <- i
	}
	close(queue)
	wg.Wait()
	if err := cache.WriteJSON(filepath.Join(output, "results.json"), results); err != nil {
		return results, err
	}
	// Selection is deterministic in input/rank order, never in completion order.
	for _, r := range results {
		if r.Passed {
			return results, nil
		}
	}
	return results, fmt.Errorf("no candidate passed; see %s", output)
}
