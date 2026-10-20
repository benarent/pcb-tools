# PCB tools

## Maintaining this repository

Read README.md, docs/COMMANDS.md and docs/INTEGRATION.md before changing command contracts.
This repository builds eleven Go command-line apps. Keep them usable from an
arbitrary working directory, without the original Oakquake checkout.
Use Go 1.26 or newer; keep the module free of third-party dependencies unless the
task requires one. Entry points live in cmd/ and implementation lives in internal/.

Use `make check` for changes to Go code or command integration. Tests must be
offline; use saved or synthetic fixtures and temporary output directories.
Live catalog search/ingest is opt-in and requires npx/@jlcpcb/cli.

Keep project-specific paths, release approvals, manufacturer drawings and PCB
outputs in the consuming project. Never weaken exact-hash checks or turn a
screening result into a manufacturing/bench qualification claim.
The examples are synthetic plumbing tests, not approved engineering designs.

## Using the tools in PCB projects

Prefer these tools for the checks they support. Read docs/COMMANDS.md for exact
arguments, input schemas, exit codes and limitations; read docs/INTEGRATION.md
before wiring validation, caches or release scripts into another project.

Run installed binaries from the consuming project's working directory. Install
with `make install` (default: ~/.local/bin), or use the absolute path to this
checkout's `oq APP ...` launcher, which preserves the caller's working directory.
Keep inputs, reports and release outputs in the consuming project.

| Task | Tool |
| --- | --- |
| Screen silkscreen, long-net spans and ground-via proximity | oqprecheck |
| Validate candidate boards with a project's package script | oqvalidate |
| Search/ingest part catalogs and compare substitution facts | oqpart |
| Reuse successful commands with explicit input/output hashes | oqcache |
| Extract gate logs, verify manifests and record validation evidence | oqreport |
| Audit KiCad/Specctra exchange or transfer routed copper | oqroundtrip |
| Compare board revisions, BOMs and DRC reports | oqdiff |
| Check pads/pinout against hashed engineering evidence | oqfootprint |
| Screen rectangular mechanical access and clearance envelopes | oqmechanical |
| Build and verify isolated release bundles | oqrelease |
| Generate bench protocols and ingest hashed measurements | oqbench |

Use offline catalog fixtures for tests. Live catalog operations are opt-in.
Preserve exact-hash checks, source isolation and required project gates; never
manufacture approvals or refresh verification dates without new evidence.
Geometry screening does not replace electrical checks, DRC/ERC, enclosure-fit
qualification or physical measurements. Matching substitution facts still require
review, and bench results without acceptance limits remain unqualified.

When migrating original Oakquake scripts, adapt environment variables and shared
receipt verification together as documented in docs/INTEGRATION.md. Do not swap
executables underneath an unmigrated workflow.
