# AGENTS.md — working on domovoi with an LLM

This file is for coding agents. Humans start at [README.md](README.md); the
design, and the reasons behind every decision, live in [DESIGN.md](DESIGN.md),
which is the source of truth — when code and DESIGN.md disagree, say so and
fix one of them, never silently pick.

## What this is

One Go binary for a Raspberry Pi Zero 2W with a 7.5" three-colour e-ink
panel: it polls smart-home sources (Tuya cloud, Aqara MCP, Open-Meteo),
keeps the state of the home, records history in SQLite and paints scenes
onto displays. Everything is pure Go, no cgo, `go 1.25.0` in go.mod — keep it
there: pin dependency versions that still build on 1.25 (check the
`GoVersion` of a module before `go get`, e.g. `goose v3.27.0`,
`modernc.org/sqlite v1.59.0`, `x/image v0.45.0`).

## Commands

```sh
make test              # go test -race ./...
make test-integration  # plus //go:build integration tests (SQLite files, sockets)
make lint              # golangci-lint v2.12.2 with --build-tags integration; must be 0 issues
make generate          # gomock mocks from every interfaces.go (go tool mockgen)
make release           # arm64 binary for the Pi
go test ./internal/scene -update   # regenerate golden frames after an intended visual change
```

Run all four checks (unit, integration, lint, `go build ./...`) before
declaring anything done. `go generate ./... && go mod tidy` must leave the
tree unchanged; CI diffs it.

## Architecture in five lines

- `cmd/domovoi` is ~50 lines; `internal/application` assembles everything:
  `enabled()` lists the modules, `run.go` wires sources, pollers, displays
  and coordinators, `application/modules/*.go` is the only glue between YAML
  config, the container and domain packages.
- `internal/container` holds lazy singletons (`Lazy[T]`) and kind
  registries; a `Module` has `Register/Start/Stop`, optionally `Assemble`.
- Domain packages (`model`, `source/*`, `state`, `storage/*`, `scene`,
  `render`, `core`, `display/*`) never import `config`, `container` or
  `application`; they take plain `Config` structs and interfaces.
- Data flow: `Source.Poll` → `Poller` → `core` (`Sink`) → `state.Apply` →
  events via the metric catalogue → `store.Persist` (one transaction per
  batch) → render coordinators (tick-driven) → `Display.Show`.
- `internal/model` is the canonical vocabulary: `Device`, `Reading`,
  `Value`, `Event`, the metric catalogue with units and event transitions.
  A metric exists only when a scene or a rule consumes it.

## Conventions (enforced by review and lint)

- Every package's dependencies are interfaces in its own `interfaces.go`,
  with a `//go:generate go tool mockgen` line producing `mocks_test.go`.
  Unit tests use only those mocks. Metrics are such an interface too
  (`IncPollSuccess`, `IncRender`…); only `internal/metrics` imports
  Prometheus. The one deliberate exception is `internal/storage`: the
  repositories and the `store` unit of work are exercised against a real
  SQLite file in integration tests and mock nothing (DESIGN.md, Testing).
- Declaration order in a file: struct → consts → vars → helper types →
  constructor → public methods → private. Wrap every error with context
  (`fmt.Errorf("tuya: list devices: %w", err)`). Doc comments on all
  exported identifiers, package comment in `doc.go`.
- Tests: table-driven with `name`/`want`/`wantErr`, one test per public
  method named `TestTypeMethod`. Time is virtual: `testing/synctest`
  bubbles (`synctest.Test`), never injected clocks; `t.Run` inside a
  bubble panics — open the bubble inside each subtest. Anything touching
  files, databases or sockets is an integration test in
  `*_integration_test.go` under `//go:build integration`, happy path only,
  no simulated DB failures. Repository packages get one such file each.
- Modern Go only: `wg.Go`, `slices`/`maps`, `for i := range n`,
  `t.Context()`, `cmp.Or`; the `modernize`, `intrange` and `usetesting`
  linters keep it so.
- Prefer an established library to hand-written protocol code
  (Prometheus client, go-i18n, goose, go-systemd) — measure the arm64
  binary cost and note it in DESIGN.md's dependency table.
- All repository content is English: code, comments, docs, commit
  messages. User-facing strings go through `internal/i18n` catalogues
  (`locales/en.yaml`, `locales/ru.yaml`) with named placeholders; a test
  checks key parity and that every event kind, weather condition and
  alert severity has a message.
- Scenes are golden-tested (`internal/scene/testdata/*.png`). After a
  deliberate visual change regenerate with `-update` and look at the PNGs.
- `-check` touches nothing outside the process: a module whose singleton
  or instances would create a file or open hardware looks at
  `container.Check` and validates instead (the storage module checks the
  directory and migrates an in-memory database). Tests never rely on the
  working directory for output paths: `t.TempDir()` everywhere.
- Never commit, push or tag unless the user asks in that turn. Never
  change the global git config; identity is set per repository.
- Secrets never enter the repo, the chat or the shell history: config uses
  `${NAME}` placeholders resolved from `$CREDENTIALS_DIRECTORY`,
  `/run/secrets` or the environment; local runs source a gitignored `.env`.

## Adding things

- **A source**: `internal/source/<kind>/` with `Config`, `New(cfg, client,
  log, metrics)`, `Poll` returning `source.Batch`; glue in
  `application/modules/<kind>.go` decoding the YAML section strictly and
  registering the kind; one line in `enabled()`; a section in
  `configs/domovoi.example.yaml`; a paragraph under "Adapters" in DESIGN.md.
- **A display**: `internal/display/<kind>/` implementing `display.Display`
  (Show honours ctx; Show and Close serialised); glue and registration as
  above; the glue must not open the hardware when `c.Check` is set.
- **A metric**: a constant and a catalogue entry in `internal/model`, the
  vendor mapping in the adapter, the widget that shows it, i18n keys if it
  needs words.
- **A schema change**: a new `internal/storage/sqlite/migrations/NNNNN_*.sql`
  with `-- +goose Up` only (forward-only), Postgres-friendly SQL
  (`ON CONFLICT DO UPDATE`, dates computed in Go, `?` placeholders).
