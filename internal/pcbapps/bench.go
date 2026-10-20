package pcbapps

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type BenchCheck struct {
	TimeColumn  string   `json:"time_column,omitempty"`
	ID          string   `json:"id"`
	Requirement string   `json:"requirement"`
	Procedure   string   `json:"procedure"`
	Unit        string   `json:"unit"`
	Column      string   `json:"column"`
	Statistic   string   `json:"statistic"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	MinSamples  int      `json:"min_samples"`
	SampleRate  float64  `json:"sample_rate_hz,omitempty"`
	LowHz       float64  `json:"low_hz,omitempty"`
	HighHz      float64  `json:"high_hz,omitempty"`
}
type BenchPlan struct {
	Schema int          `json:"schema"`
	Title  string       `json:"title"`
	Checks []BenchCheck `json:"checks"`
}
type Capture struct {
	Check  string `json:"check"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Unit   string `json:"unit"`
}
type BenchRun struct {
	Schema      int       `json:"schema"`
	Serial      string    `json:"board_serial"`
	PCB         string    `json:"pcb_sha256"`
	Firmware    string    `json:"firmware_sha256"`
	Measured    string    `json:"measured_utc"`
	Instruments []string  `json:"instruments"`
	Captures    []Capture `json:"captures"`
}
type SpectrumBin struct {
	Hz  float64 `json:"hz"`
	PSD float64 `json:"power_per_hz"`
}
type Measurement struct {
	ID         string        `json:"id"`
	Unit       string        `json:"unit"`
	Value      float64       `json:"value"`
	Samples    int           `json:"samples"`
	Status     string        `json:"status"`
	CaptureSHA string        `json:"capture_sha256"`
	Spectrum   []SpectrumBin `json:"spectrum,omitempty"`
}
type BenchReport struct {
	Status       string        `json:"status"`
	PlanSHA      string        `json:"plan_sha256"`
	Run          BenchRun      `json:"run"`
	Measurements []Measurement `json:"measurements"`
	Issues       []string      `json:"issues"`
}

func validBench(p BenchPlan) error {
	if p.Schema != 1 || p.Title == "" || len(p.Checks) == 0 {
		return fmt.Errorf("nonempty schema 1 bench plan required")
	}
	seen := map[string]bool{}
	for _, c := range p.Checks {
		if c.ID == "" || seen[c.ID] || c.Requirement == "" || c.Procedure == "" || c.Column == "" || c.Unit == "" || c.MinSamples < 1 || (c.Min != nil && c.Max != nil && *c.Min > *c.Max) {
			return fmt.Errorf("invalid/duplicate check %s", c.ID)
		}
		seen[c.ID] = true
		switch c.Statistic {
		case "min", "max", "mean", "rms", "stddev", "peak_to_peak", "sum":
		case "band_rms":
			if c.TimeColumn == "" || c.SampleRate <= 0 || c.LowHz < 0 || c.HighHz <= c.LowHz || c.HighHz > c.SampleRate/2 {
				return fmt.Errorf("invalid spectral band %s", c.ID)
			}
		default:
			return fmt.Errorf("unknown statistic %s", c.Statistic)
		}
	}
	return nil
}
func Bench(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: oqbench procedure plan.json protocol.md | oqbench ingest plan.json run.json report.json")
	}
	var p BenchPlan
	if e := ReadJSON(args[1], &p); e != nil {
		return e
	}
	if e := validBench(p); e != nil {
		return e
	}
	if args[0] == "procedure" && len(args) == 3 {
		var s strings.Builder
		fmt.Fprintf(&s, "# %s\n\nRecord board serial, PCB SHA-256, firmware SHA-256, UTC measurement time, instruments and raw capture hashes. No measurements are implied by this procedure.\n", p.Title)
		for _, c := range p.Checks {
			fmt.Fprintf(&s, "\n## %s\n\nRequirement: %s\n\n%s\n\nCapture CSV column `%s` in `%s`; at least %d samples. Statistic: `%s`.\n", c.ID, c.Requirement, c.Procedure, c.Column, c.Unit, c.MinSamples, c.Statistic)
			if c.Min == nil && c.Max == nil {
				s.WriteString("Acceptance: not specified; measurement will remain unqualified.\n")
			} else {
				if c.Min != nil {
					fmt.Fprintf(&s, "Minimum: %g %s.\n", *c.Min, c.Unit)
				}
				if c.Max != nil {
					fmt.Fprintf(&s, "Maximum: %g %s.\n", *c.Max, c.Unit)
				}
			}
		}
		return os.WriteFile(args[2], []byte(s.String()), 0644)
	}
	if args[0] != "ingest" || len(args) != 4 {
		return fmt.Errorf("invalid bench command")
	}
	var run BenchRun
	if e := ReadJSON(args[2], &run); e != nil {
		return e
	}
	r, e := IngestBench(p, run, filepath.Dir(args[2]))
	if e != nil {
		return e
	}
	r.PlanSHA, e = Hash(args[1])
	if e != nil {
		return e
	}
	fmt.Printf("bench measurements: %s (%d checks, %d findings)\n", r.Status, len(r.Measurements), len(r.Issues))
	return SaveReport(args[3], r, r.Issues)
}
func IngestBench(p BenchPlan, run BenchRun, base string) (BenchReport, error) {
	r := BenchReport{Status: "passed", Run: run, Measurements: []Measurement{}, Issues: []string{}}
	if e := validBench(p); e != nil {
		return r, e
	}
	if run.Schema != 1 || run.Serial == "" || !hashValid(run.PCB) || !hashValid(run.Firmware) || len(run.Instruments) == 0 {
		return r, fmt.Errorf("run must identify serial, PCB/firmware hashes and instruments")
	}
	date, e := time.Parse(time.RFC3339, run.Measured)
	if e != nil || date.After(time.Now().Add(time.Minute)) {
		return r, fmt.Errorf("invalid/future measurement timestamp")
	}
	cap := map[string]Capture{}
	ids := map[string]bool{}
	for _, c := range p.Checks {
		ids[c.ID] = true
	}
	for _, c := range run.Captures {
		if cap[c.Check].Check != "" || !ids[c.Check] || !SafeRelative(c.Path) || !hashValid(c.SHA256) {
			return r, fmt.Errorf("invalid/duplicate capture %s", c.Check)
		}
		cap[c.Check] = c
	}
	for _, c := range p.Checks {
		v, ok := cap[c.ID]
		if !ok {
			r.Issues = append(r.Issues, "missing capture: "+c.ID)
			continue
		}
		if v.Unit != c.Unit {
			return r, fmt.Errorf("unit mismatch for %s", c.ID)
		}
		path := filepath.Join(base, v.Path)
		h, e := Hash(path)
		if e != nil {
			return r, e
		}
		if h != v.SHA256 {
			return r, fmt.Errorf("capture hash mismatch %s", v.Path)
		}
		samples, e := readCSV(path, c.Column)
		if e != nil {
			return r, e
		}
		if len(samples) < c.MinSamples {
			r.Issues = append(r.Issues, "insufficient samples: "+c.ID)
			continue
		}
		if c.Statistic == "band_rms" {
			times, e := readCSV(path, c.TimeColumn)
			if e != nil {
				return r, e
			}
			if len(times) != len(samples) {
				return r, fmt.Errorf("timestamp/sample count differs")
			}
			for i := 1; i < len(times); i++ {
				if math.Abs((times[i]-times[i-1])*c.SampleRate-1) > .001 {
					return r, fmt.Errorf("nonuniform/wrong sample rate: %s", c.ID)
				}
			}
		}
		value, spectrum, e := statistic(samples, c)
		if e != nil {
			return r, e
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return r, fmt.Errorf("nonfinite measurement")
		}
		m := Measurement{ID: c.ID, Unit: c.Unit, Value: value, Samples: len(samples), Status: "passed", CaptureSHA: h, Spectrum: spectrum}
		if c.Min == nil && c.Max == nil {
			m.Status = "unqualified"
			r.Status = "unqualified"
		}
		if (c.Min != nil && value < *c.Min) || (c.Max != nil && value > *c.Max) {
			m.Status = "failed"
			r.Issues = append(r.Issues, c.ID+": outside acceptance limits")
		}
		r.Measurements = append(r.Measurements, m)
	}
	if len(r.Issues) > 0 {
		r.Status = "failed"
	}
	return r, nil
}
func hashValid(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func readCSV(path, column string) ([]float64, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	rows, e := csv.NewReader(f).ReadAll()
	if e != nil {
		return nil, e
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("capture has no samples")
	}
	idx := -1
	for i, k := range rows[0] {
		if k == column {
			if idx >= 0 {
				return nil, fmt.Errorf("duplicate CSV column")
			}
			idx = i
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("missing column %s", column)
	}
	a := make([]float64, 0, len(rows)-1)
	for _, row := range rows[1:] {
		v, e := strconv.ParseFloat(strings.TrimSpace(row[idx]), 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("invalid measurement sample")
		}
		a = append(a, v)
	}
	return a, nil
}
func statistic(a []float64, c BenchCheck) (float64, []SpectrumBin, error) {
	lo, hi, sum, squares := a[0], a[0], 0., 0.
	for _, v := range a {
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
		sum += v
		squares += v * v
	}
	mean := sum / float64(len(a))
	switch c.Statistic {
	case "min":
		return lo, nil, nil
	case "max":
		return hi, nil, nil
	case "sum":
		return sum, nil, nil
	case "mean":
		return mean, nil, nil
	case "rms":
		return math.Sqrt(squares / float64(len(a))), nil, nil
	case "stddev":
		var d float64
		for _, v := range a {
			d += (v - mean) * (v - mean)
		}
		return math.Sqrt(d / float64(len(a))), nil, nil
	case "peak_to_peak":
		return hi - lo, nil, nil
	case "band_rms":
		n := len(a)
		if n < 8 || n&(n-1) != 0 || n > 65536 {
			return 0, nil, fmt.Errorf("spectrum requires 8..65536 samples, power of two, uniformly sampled")
		}
		x := make([]complex128, n)
		var energy float64
		for i, v := range a {
			w := .5 - .5*math.Cos(2*math.Pi*float64(i)/float64(n))
			x[i] = complex((v-mean)*w, 0)
			energy += w * w
		}
		fft(x)
		var bins []SpectrumBin
		power := 0.
		df := c.SampleRate / float64(n)
		for i := 0; i <= n/2; i++ {
			v := cmplx.Abs(x[i])
			psd := v * v / (c.SampleRate * energy)
			if i > 0 && i < n/2 {
				psd *= 2
			}
			hz := float64(i) * df
			bins = append(bins, SpectrumBin{hz, psd})
			if hz >= c.LowHz && hz <= c.HighHz {
				power += psd * df
			}
		}
		return math.Sqrt(power), bins, nil
	}
	return 0, nil, fmt.Errorf("unknown statistic")
}
func fft(a []complex128) {
	n := len(a)
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for j&bit != 0 {
			j ^= bit
			bit >>= 1
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := a[start+k], a[start+k+size/2]*w
				a[start+k], a[start+k+size/2] = u+v, u-v
				w *= step
			}
		}
	}
}
