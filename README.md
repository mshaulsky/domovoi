# domovoi

A smart-home dashboard for a Raspberry Pi Zero 2W and an e-ink panel, in
one Go binary: it polls the sources, keeps the state of the home, and
paints it onto one or more displays. Named after the domovoi, the Slavic
house spirit that quietly keeps watch over the household.

Status: **stage 1** — the assembly, the Tuya source, the PNG display and the
overview scene. It runs on a PC and draws real Tuya data to a PNG. The
e-ink backend, the Aqara and weather sources, history, alerts, Telegram and
the back office follow; the design is in [DESIGN.md](DESIGN.md).

## Run it

```sh
cp domovoi.env.example .env          # fill in the Tuya credentials
cp configs/domovoi.example.yaml domovoi.yaml
. .env && go run ./cmd/domovoi -config domovoi.yaml -check   # validate
. .env && go run ./cmd/domovoi -config domovoi.yaml -once    # one frame → frame.png
. .env && go run ./cmd/domovoi -config domovoi.yaml          # keep rendering every tick
```

Flags: `-config`, `-check`, `-once`, `-verbose`, `-version`.

Secrets never live in the YAML: `${NAME}` placeholders resolve from a file
`$CREDENTIALS_DIRECTORY/NAME` (systemd `LoadCredential=`), then
`/run/secrets/NAME`, then the environment. `deploy/systemd/domovoi.service`
shows the Pi setup: the unit is `Type=notify`, the binary reports readiness
and feeds `WatchdogSec=` itself.

## Develop

```sh
make test              # unit tests, race detector
make test-integration  # plus the tests that touch files and sockets
make lint              # golangci-lint at the pinned version
make generate          # mocks from every interfaces.go
make icons             # regenerate the icon font subset (needs node + subset-font)
make release           # the arm64 binary for the Pi
```

Scenes are tested against golden images in `internal/scene/testdata`; after
an intended visual change run `go test ./internal/scene -update` and review
the PNGs. Time-dependent code is tested under `testing/synctest`: no clock
injection, virtual time in the tests.

## Layout

- `cmd/domovoi` — flags and signals, nothing else.
- `internal/application` — modules, their lifecycle, `Run`; `application/modules` is the glue between domain packages and the container.
- `internal/container` — lazy singletons and kind registries.
- `internal/model` — the canonical readings, events and the metric catalogue.
- `internal/source` — the source contract and the poller; `source/tuya` the first adapter.
- `internal/state` — what is true now, source health, event classification.
- `internal/render` — one coordinator per display: tick-driven rendering.
- `internal/scene` — widgets, layout, theme; `internal/icon` the pictograms; `internal/i18n` the catalogues (en, ru).
- `internal/display` — the display contract; `display/pngfile` the first backend.
- `internal/config`, `internal/metrics` — configuration with secrets, counters.

## Licence

MIT. The icon font subset is derived from
[Material Design Icons](https://pictogrammers.com/library/mdi/) (Apache 2.0,
see `internal/icon/LICENSE-MDI`); the text faces are the Go fonts.
