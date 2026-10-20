.DEFAULT_GOAL := help
GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
APPS := $(notdir $(wildcard cmd/oq*))

.PHONY: help build test check install clean
help:
	@printf '%s\n' 'make build    Build all eleven apps into bin/' \
	  'make test     Run Go tests with the race detector' \
	  'make check    Run tests, vet, and portable CLI smoke tests' \
	  'make install  Install into ~/.local/bin (override PREFIX or BINDIR)' \
	  './oq APP ...  Build and run one app, preserving your working directory'

build:
	@mkdir -p bin
	$(GO) build -o bin/ ./cmd/...

test:
	$(GO) test -race ./...

check: test build
	$(GO) vet ./...
	bash scripts/smoke.sh

install: build
	@mkdir -p "$(BINDIR)"
	@for app in $(APPS); do install -m 755 "bin/$$app" "$(BINDIR)/$$app"; done

clean:
	rm -rf bin
