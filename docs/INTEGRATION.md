# Integrating a PCB project

Keep source boards, manufacturer evidence, contracts, package scripts and reviewed
release records in the consuming project. Run these apps with that project as the
working directory. Paths are relative to the working directory unless stated below.

## Candidate validation

Supply `oqvalidate -script package.sh -out new-results board-a.kicad_pcb board-b.kicad_pcb`.
The script receives `candidate INPUT OUTPUT`. INPUT is a private copy; OUTPUT is a
new per-candidate directory. Create OUTPUT, run all applicable gates, and return
nonzero on any failure. Never write candidate results into shared canonical exports.
The runner uses Bash, captures logs and writes results.json. Selection follows input
order, not completion time; exit 0 means at least one candidate passed your script.
The runner does not validate PCB contents by itself.

To reuse source-only checks within a batch, add `-shared-spec dependencies.json`.
The script first receives `check`, with `OQ_CHECK_OUTPUT` naming a new artifact
directory to create. Candidate calls receive `OQ_SHARED_CHECK`, a receipt path.
Before reusing source artifacts, resolve and verify that receipt:

```sh
shared="$(oqvalidate verify-shared "$OQ_SHARED_CHECK")"
# Copy needed source artifacts from "$shared" into this candidate's output.
```

The runner also verifies dependencies and artifacts before/after the batch.
Candidate-specific copper, netlist, DRC and release checks must still run.
The shared check's output is immutable during candidate validation.

The dependency specification is explicit:

```json
{"paths":["src","bom","package.sh","contracts"],"values":{"kicad":"your exact version","config":"your settings"}}
```

Include all source imports, catalog/model/evidence files, scripts, tool versions
and relevant environment values. These are not discovered automatically. Do not
include output/cache directories or the entire working tree containing them.
Directory hashes include file names, contents and modes, plus symlink targets and
contents; only `.DS_Store` and `__pycache__` entries are skipped.

For persistent reuse across batches:

```sh
cached="$(oqcache run dependencies.json .cache/preflight -- \
  env 'OQ_CHECK_OUTPUT={cache}' bash package.sh check)"
oqvalidate -script package.sh -shared-spec dependencies.json \
  -preflight-cache "$cached" -out new-results board-a.kicad_pcb board-b.kicad_pcb
```

Use the exact script argument in both commands. `oqcache` fingerprints the literal
argv, and `oqvalidate` verifies that it was the expected source check. Failed commands,
changed dependencies or changed output files cannot be reused. A cache hit does not
refresh stock/price dates.

## Release packaging

Use `oqrelease build release-plan.json new-bundle`. The plan defines:

```json
{
  "schema":1,
  "snapshot_paths":["src","bom","contracts","package.sh","review.json"],
  "command":["bash","package.sh","release"],
  "manifest":"out/manifest.txt",
  "pcb":"out/board.kicad_pcb",
  "review":"review.json",
  "required_checks":["layout"],
  "extras":["out/VALIDATION.md"]
}
```

All plan paths are relative to the project root. Explicitly include everything the
build needs, including installed dependencies when applicable. The command runs
inside a private snapshot with `OQ_RELEASE_STAGE=1`; this lets a wrapper call its
actual packaging steps without recursively invoking oqrelease.

The build must exit 0, and produce a nonempty SHA-256 manifest and a review record
binding `pcb_sha256` to the exact finished PCB. Every required check (and every
additional listed check) must have `status:"verified"` and nonempty `evidence`.
PCB and review must be in the manifest. Optional review `artifact_sha256` entries
bind reviewed models/evidence to both snapshot inputs and published files.
Do not generate a real layout approval automatically in the build.

Files are verified and the complete directory is renamed into place. Existing
targets are refused. On failure no bundle is published; logs and a `.failed`
workspace remain beside the requested target. `oqrelease verify BUNDLE` detects
changed, missing and unlisted files. Receipts are not signatures, and bundles are
immutable by convention rather than filesystem enforcement. Package commands are
trusted configuration, not sandboxed programs.

## Compatibility with the original Oakquake tools

Executable names and data contracts are retained. Deliberate integration changes:

| Original | Standalone |
| --- | --- |
| Default v2.4 package script | Required explicit `oqvalidate -script` |
| `OAKQUAKE_V24_OUTPUT` | `OQ_CHECK_OUTPUT` |
| `OAKQUAKE_SHARED_CHECK` | `OQ_SHARED_CHECK` |
| `OAKQUAKE_RELEASE_STAGE` | `OQ_RELEASE_STAGE` |
| Project JavaScript parts helper | Native Go ingestion |
| Implicit exclusion of `tools/go/bin` from hashes/copies | Explicitly enumerate source inputs |
| Fixed report prefixes | Compatibility defaults or `-prefixes prefixes.json` |
| Fixed GND/2.5 mm/50 mm precheck settings | Configurable flags with the same defaults |

The original Oakquake checkout keeps its existing tools and scripts. Do not point
its unmodified scripts at these new binaries: migrate environment names and shared
receipt verification together, or use a project-specific adapter.
