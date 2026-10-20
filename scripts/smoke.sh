#!/usr/bin/env bash
# Exercise built binaries from a different project directory, entirely offline.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/pcb-tools-smoke.XXXXXX")"
trap 'rm -rf "$work"' EXIT
cp -R "$ROOT/examples/minimal" "$work/project with spaces"
export PATH="$ROOT/bin:$PATH"
cd "$work/project with spaces"
mkdir reports

oqprecheck geometry.json drc.json reports/precheck.json
oqroundtrip -mode check board.kicad_pcb board.kicad_pcb reports/roundtrip.json
oqdiff -svg reports/overlay.svg board.kicad_pcb board.kicad_pcb reports/diff.json
oqfootprint check footprints.json board.kicad_pcb reports/footprints.json
oqmechanical check mechanical.json board.kicad_pcb reports/mechanical.json
oqpart ingest circuit.json reports/parts.json catalog --offline
oqpart compare part.json part.json reports/comparison.json
oqbench procedure bench-plan.json reports/protocol.md
oqbench ingest bench-plan.json run.json reports/bench.json
grep -q '"status": "unqualified"' reports/bench.json
oqreport gates -prefixes prefixes.json package.log > reports/gates.txt
test "$(cat reports/gates.txt)" = "fixture gate: synthetic pass"
oqreport manifest manifest.txt .
printf '# Synthetic report\n' > reports/VALIDATION.md
oqreport validation -prefixes prefixes.json package.log manifest.txt reports/VALIDATION.md .
grep -q 'fixture gate: synthetic pass' reports/VALIDATION.md
# The source launcher must preserve cwd too; relative files exist only here.
"$ROOT/oq" oqreport manifest manifest.txt .

oqvalidate -script package.sh -shared-spec dependencies.json -jobs 2 \
  -out validation board.kicad_pcb board.kicad_pcb
cached="$(oqcache run dependencies.json cache -- env 'OQ_CHECK_OUTPUT={cache}' bash package.sh check)"
again="$(oqcache run dependencies.json cache -- env 'OQ_CHECK_OUTPUT={cache}' bash package.sh check 2>reports/cache-hit.log)"
test "$cached" = "$again"
grep -q 'cache: HIT' reports/cache-hit.log
oqvalidate -script package.sh -shared-spec dependencies.json -preflight-cache "$cached" \
  -out cached-validation board.kicad_pcb

oqrelease build release-plan.json "$work/bundle"
oqrelease verify "$work/bundle"

# Negative checks: cached/approved results must not survive modified artifacts.
printf 'tampered\n' >> "$cached/source.kicad_pcb"
if oqvalidate -script package.sh -shared-spec dependencies.json -preflight-cache "$cached" \
  -out invalid-cache board.kicad_pcb > reports/cache-rejection.log 2>&1; then
  printf 'ERROR: modified cache accepted\n' >&2; exit 1
fi
printf 'tampered\n' >> "$work/bundle/board.kicad_pcb"
if oqrelease verify "$work/bundle" > reports/release-rejection.log 2>&1; then
  printf 'ERROR: modified bundle accepted\n' >&2; exit 1
fi
printf '%s\n' 'Portable CLI smoke: all 11 apps passed; cache and release tampering rejected.'
