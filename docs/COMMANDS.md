# Command reference

Flags precede positional arguments. Geometry uses millimetres. Footprint,
mechanical, release and bench JSON contracts use schema 1 and reject unknown fields.
See `examples/minimal/` for complete runnable fixtures. Reports are JSON unless noted.
Most gate findings exit 1; invalid CLI usage exits 1 or 2.

## Pre-routing

```sh
oqprecheck -ground-net GND -ground-radius-mm 2.5 -long-net-mm 50 \
  geometry.json drc.json precheck.json
```

Geometry has `pads:[{ref,pin,net,x,y}]`, `ground_vias:[{x,y}]` and
`required:["U1.1"]`. DRC input has `violations:[{type,severity,description}]`.
The script exporting geometry must supply ground vias/anchors from the actual
prepared board. This app checks existing reservations; it does not insert vias.
It rejects silk_over_copper, silk_overlap and silk_over_mask violations, and missing
local ground reservations. Long-net output is maximum pin-to-pin span, a lower bound
on routing length. Proximity does not prove connectivity; final DRC remains required.

## Validation and caching

```sh
oqvalidate -script package.sh -jobs 2 -out new-results \
  candidate-a.kicad_pcb candidate-b.kicad_pcb
oqvalidate -script package.sh -shared-spec dependencies.json \
  -out new-results candidate.kicad_pcb
oqvalidate verify-shared receipt.json
oqcache key dependencies.json
oqcache run dependencies.json .cache/preflight -- command '{cache}'
```

See [integration](INTEGRATION.md) for script arguments, environment variables, cache
adoption and dependency inventory. Outputs must be isolated and return nonzero on
failure. `{cache}` expands to a fresh directory which the command must create.
Cache stdout is just its reusable output path; HIT/MISS diagnostics go to stderr.

## Parts

```sh
oqpart search C12345
oqpart search "ADC differential"
oqpart ingest source.circuit.json parts.json catalog
oqpart ingest source.circuit.json parts.json catalog --offline
oqpart compare original-part.json candidate-part.json comparison.json
```

Live operations call `npx @jlcpcb/cli search QUERY --json`; no desktop quote/upload
or ordering actions are performed. Ingest reads tscircuit `source_component` and
`pcb_component` records, ignores do_not_place parts and test points, and resolves
the first JLCPCB supplier ID for each placed source component. IDs must be C followed
by digits. Unused parts are removed from the resulting parts array.

Saved catalog records are `catalog/C12345.json`: `lcsc_id`, `name`, `package`,
`library_type` (basic/extended), nonnegative numeric `stock` and `price`; optional
`manufacturer`, `datasheet`, `attributes`. The parts file is atomically replaced
only after every entry validates. Offline mode requires those records, preserves
prior verification dates and does not mark undated entries verified. Dates remain
historical evidence, not a new stock/price check.

Compare takes individual objects with `mpn`, `package`, `pinout`, `voltage_limits`,
`tolerance`, `temperature_rating`, `assembly_requirements` and `attributes`.
Missing facts or differences require review and exit 1. Equal facts exit 0 but do
not authorize a substitution. Units/conventions must agree; no BOM/source is changed.

## Reports

```sh
oqreport gates [-prefixes prefixes.json] package.log
oqreport manifest manifest.txt [root]
oqreport validation [-prefixes prefixes.json] package.log manifest.txt VALIDATION.md root
```

Custom prefixes are a JSON array such as `["DRC:","electrical check:"]`, replacing
the compatibility defaults inherited from Oakquake. Validation verifies a nonempty
manifest and updates only the `<!-- oqreport:start -->` / `<!-- oqreport:end -->`
section (appending it when absent). Log extraction does not independently certify
the checks; your successful pipeline supplies provenance.

Manifest entries use lowercase SHA-256, two spaces, then a path. `oqreport` accepts
headings and ignores non-entry lines; it supports absolute paths and paths relative
to root. For strict release path/inventory checking use `oqrelease`.

## Routing integrity

```sh
oqroundtrip -mode export -layers F.Cu,B.Cu prepared.kicad_pcb board.dsn export.json
oqroundtrip transfer unrouted.kicad_pcb native.kicad_pcb candidate.kicad_pcb
oqroundtrip -mode import -ses board.ses board.dsn candidate.kicad_pcb import.json
oqroundtrip -mode check before.kicad_pcb after.kicad_pcb preservation.json
```

Export checks placements, pad nets/positions, selected layers and protected copper.
Import checks DSN/SES placement and copper against the imported board. Check requires
baseline copper and pad geometry to be preserved; use oqdiff for intentional edits.
Reports bind input hashes. Default tolerance is 0.002 mm; collinear splitting/merging
is accepted only when coverage, width, layer and net remain intact.

Transfer copies segments, arcs and vias into an unrouted source board, mapping named
nets to numeric IDs. Outer-layer buried vias become through vias. It preserves
widths and refuses pre-existing source copper, unknown nets, internal buried vias
and input/output aliases. Run the exchange audit and DRC afterward.

KiCad numeric/named nets, front-side Specctra placement, DSN units/SES resolution,
straight paths and circular vias are supported. Back-side Specctra placement fails
explicitly. Cross-format checks omit drill dimensions, keepouts and custom pad
outlines; PCB-to-PCB checks include pad dimensions/drills/custom primitives.

## Design differences

```sh
oqdiff -svg overlay.svg -bom-before before-parts.json -bom-after after-parts.json \
  -drc-before before-drc.json -drc-after after-drc.json \
  before.kicad_pcb after.kicad_pcb changes.json
```

Flags are optional; each before/after pair must be complete. Reports cover component
identity/placement, pads/nets, copper, layers, thickness, outlines and zones, with
affected gates. BOMs are parts arrays keyed by LCSC ID; DRC inputs are KiCad JSON.
3D model references/transforms are not compared. SVG shows red before/blue after,
with approximate pad rectangles and straight tracks/vias; zones/arcs are omitted.
Changes are informational (exit 0); invalid inputs exit 1.

## Footprint contracts

```sh
oqfootprint check footprints.json board.kicad_pcb report.json
```

Contract fields: `schema:1`, `status:"reviewed"`, `tolerance_mm` (0–0.05 exclusive
of zero), `footprints`. Each footprint has `ref`, `pad_count`, `pads`, `evidence`;
optional `value`, `footprint`, `rotation` constrain identity.
Pad assertions have `number`, `count`, optional `kind`, `shape`, `net`, `local`,
`size`, `drill`, `rotation`, `layers`. Coordinates/dimensions are `{x,y}`. Empty
`net` means explicitly unconnected; repeated unnumbered holes use `number:""`.
Slots use both drill dimensions; rotations are KiCad geometry, not CPL offsets.

Each evidence entry has a relative `path` (relative to the contract), `sha256`,
`revision`, and `basis`. Geometry must be transcribed from reviewed engineering
evidence; the app does not extract or approve dimensions from drawings.
Changed evidence, missing pads or mismatched assertions fail.

## Mechanical envelopes

```sh
oqmechanical check mechanical.json board.kicad_pcb report.json
oqmechanical check mechanical-source.json source.circuit.json report.json
```

Contract fields: `schema:1`, `coordinate_system` (`source-mm-y-up` or
`kicad-mm-y-down`), `bodies`, `envelopes`. Bodies have `ref`, `size`, `offset`,
`height_mm`, optional `side`. Envelopes have `id`, `center`, `size`, `rotation`,
`side` (top/bottom/both), `z_min_mm`, `margin_mm`, `allow` and hashed `evidence`.
Optional `anchor` makes coordinates/rotation relative to that component; optional
`z_max_mm` bounds height. Heights extend outward from the selected board face.

Without explicit bodies, source placement/courtyard/pad bounds provide conservative
rectangles and the report identifies their basis. Overlap uses rotated rectangles.
Unknown height in a potentially overlapping height-limited envelope fails.
This is an envelope screen, not STEP solid intersection or proof of enclosure fit.

## Release bundles

```sh
oqrelease build release-plan.json new-bundle
oqrelease verify bundle
```

See [integration](INTEGRATION.md) for plans, exact-hash reviews, isolated builds and
publication behavior. Every published file except the receipt itself is hashed.

## Bench measurements

```sh
oqbench procedure bench-plan.json protocol.md
oqbench ingest bench-plan.json run.json measurements.json
```

A plan has schema 1, title, and checks with `id`, `requirement`, `procedure`, `unit`,
`column`, `statistic`, `min_samples`, optional acceptance `min`/`max`.
A run has `board_serial`, exact `pcb_sha256` and `firmware_sha256`, past
`measured_utc`, `instruments` and captures with `check`, `path`, `sha256`, `unit`.
Capture paths are relative to the run file. Units, finite data, columns, sample
counts and hashes must match.

Statistics: min/max/mean/rms/stddev/peak_to_peak/sum/band_rms. Stddev is population
standard deviation. Band RMS uses DC removal, a periodic Hann window and a one-sided
periodogram; it requires `time_column`, `sample_rate_hz`, `low_hz`, `high_hz`,
8–65536 samples with a power-of-two count, and sample intervals within 0.1% of the
configured rate. This is a single-window estimate, not Welch averaging.

Missing/failing checks exit 1. Measurements without acceptance limits exit 0 with
status `unqualified`, never `passed`. Generated protocols and example CSVs are not
evidence of a real bench measurement.
