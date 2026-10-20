#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  check)
    mkdir "$OQ_CHECK_OUTPUT"
    cp board.kicad_pcb "$OQ_CHECK_OUTPUT/source.kicad_pcb"
    printf 'source check: synthetic fixture only\n'
    ;;
  candidate)
    mkdir "$3"
    if [ -n "${OQ_SHARED_CHECK:-}" ]; then
      shared="$(oqvalidate verify-shared "$OQ_SHARED_CHECK")"
      cmp "$shared/source.kicad_pcb" "$2"
    fi
    oqroundtrip -mode check board.kicad_pcb "$2" "$3/roundtrip.json"
    ;;
  release)
    test "${OQ_RELEASE_STAGE:-}" = 1
    mkdir checks
    oqfootprint check footprints.json board.kicad_pcb checks/footprints.json
    oqmechanical check mechanical.json board.kicad_pcb checks/mechanical.json
    oqreport manifest manifest.txt .
    printf 'fixture package: passed synthetic checks only\n'
    ;;
  *) printf 'usage: package.sh check|candidate INPUT OUTPUT|release\n' >&2; exit 2 ;;
esac
