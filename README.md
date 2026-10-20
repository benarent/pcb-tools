# PCB tools

Eleven Go command-line apps for checking PCB geometry, reviewing changes, running
candidate builds, packaging releases and recording bench results. Extracted from
Oakquake on 2026-10-20 for use in other projects. The `oq` names are retained.

## Build and use

Requires Go 1.26 or newer. The Go module has no third-party dependencies.
macOS and Linux are the supported workflow environments; validation and release
examples use Bash. KiCad/Freerouting are needed only when your own scripts invoke
them. Live catalog search and ingestion require Node/npx and `@jlcpcb/cli`;
offline ingestion is entirely Go.

```sh
git clone https://github.com/benarent/pcb-tools.git
cd pcb-tools
make build
make check

# Install all apps into ~/.local/bin (or set BINDIR=/your/bin).
make install
export PATH="$HOME/.local/bin:$PATH"
```

You can also build and run one app without installing it, from any PCB project:

```sh
~/projects/pcb-tools/oq oqdiff before.kicad_pcb after.kicad_pcb changes.json
~/projects/pcb-tools/oq oqreport manifest release-manifest.txt .
```

The launcher preserves your working directory. Installed binaries do not require
this checkout. `go install ./cmd/...` is also supported from the repository root,
using your Go environment's GOBIN/GOPATH. To install directly from GitHub:

```sh
go install github.com/benarent/pcb-tools/cmd/...@latest
```

| App | Purpose |
| --- | --- |
| `oqprecheck` | Silkscreen collisions, long-net spans and reserved ground-via proximity |
| `oqvalidate` | Parallel candidate validation with isolated outputs and shared source checks |
| `oqpart` | Live part search, atomic catalog ingestion and substitution fact comparison |
| `oqcache` | Cache successful commands against explicit input and output hashes |
| `oqreport` | Extract gate lines, verify manifests and update validation evidence |
| `oqroundtrip` | Check KiCad/Specctra exchange and transfer routed copper |
| `oqdiff` | Compare boards, optional BOM/DRC inputs and SVG overlays |
| `oqfootprint` | Check pad geometry and pinout against hashed engineering contracts |
| `oqmechanical` | Check rectangular access/clearance envelopes |
| `oqrelease` | Build in isolation and publish a verified bundle atomically |
| `oqbench` | Generate procedures and ingest hashed CSV measurements |

See [command reference](docs/COMMANDS.md) and [project integration](docs/INTEGRATION.md).
The [example project](examples/minimal/README.md) exercises all eleven binaries
offline. `make check` copies it into a temporary directory outside this repository,
runs the apps there, checks failure paths and removes the temporary outputs.

These are engineering checks and workflow helpers. Your project supplies electrical,
DRC/ERC, manufacturing and review gates. Geometry screening is not enclosure-fit
qualification, and synthetic examples are not manufacturer evidence or bench results.

## Repository

`cmd/` holds the executable entry points; `internal/` holds the shared implementation.
`bin/` and `.work/` are ignored. `make test` runs race-enabled Go tests; `make check`
also builds every app, runs `go vet`, and tests the command workflow.

Source: [github.com/benarent/pcb-tools](https://github.com/benarent/pcb-tools).
The Go module path is `github.com/benarent/pcb-tools`. Licensed under the
[MIT License](LICENSE).

Agent guidance for maintaining and using these tools is in [AGENTS.md](AGENTS.md).
