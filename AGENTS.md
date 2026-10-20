# PCB tools

Read README.md and docs/COMMANDS.md before changing command contracts.
This repository builds eleven Go command-line apps. Keep them usable from an
arbitrary working directory, without the original Oakquake checkout.

Use `make check` for changes to Go code or command integration. Tests must be
offline; use saved or synthetic fixtures and temporary output directories.
Live catalog search/ingest is opt-in and requires npx/@jlcpcb/cli.

Keep project-specific paths, release approvals, manufacturer drawings and PCB
outputs in the consuming project. Never weaken exact-hash checks or turn a
screening result into a manufacturing/bench qualification claim.
The examples are synthetic plumbing tests, not approved engineering designs.
