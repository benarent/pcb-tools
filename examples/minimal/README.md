# Synthetic workflow fixture

This tiny project demonstrates data formats and script integration. Its board,
catalog record, contracts, review and CSV are **synthetic test data**. The reviewed
status values exercise parsers only and do not represent hardware approval.
It is not a fabrication-ready PCB.

Run `make check` at the repository root for an isolated, offline demonstration.
The smoke script copies this directory elsewhere and executes all eleven apps.
Its package.sh only checks fixture identity and geometry; replace it with your
own real source, electrical, KiCad, manufacturing and release gates.
