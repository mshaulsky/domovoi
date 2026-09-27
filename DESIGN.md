# domovoi — design

A single Go binary for a Raspberry Pi Zero 2W that polls smart-home sources,
keeps history in SQLite, evaluates alerts and paints ambient dashboards onto
one or more displays. Pure Go, no cgo, cross-compiled from the PC.

Named after the domovoi, the Slavic house spirit that quietly keeps watch over
the household and grumbles when something is wrong.

Module `github.com/mshaulsky/domovoi`, binary `domovoi`.

## Goals

1. **Sources plug in with minimal effort.** A new vendor, hub or sensor kind is
   one new package that maps its native data into the canonical reading model,
   plus one line in the constructor table and one section in the config.
   Existing today: `tuyacloud` (climate sensors), `aqaramcp` (lock, leak
   sensors, outlet, button presence). Next: Zigbee2MQTT over MQTT from the
   SLZB-06U. Later: virtual sources (weather, derived metrics, presence,
   system health) and a text advisor backed by the local LLM.
2. **Displays are interchangeable and can be several.** The Waveshare 7.5" B V2
   today; unknown panels tomorrow; a PNG file and a browser page from day one.
   Scenes are laid out for whatever size and palette the display declares.
3. **Ambient, not realtime.** Full colour refresh costs ~19 s, partial mono
   ~1.5 s, and the panel should not see a full refresh more often than every
   ~3 min. The **refresh budget** is a product requirement, not a side effect
   of timings: one render per tick (default 5 min), one full refresh per hour,
   and an out-of-band render only when an alert changes. Scenes never know
   how a frame reaches the glass.
4. **Red is for alerts.** Alerts are first-class: rules produce alert state,
   which drives the red plane, forces a colour refresh and fans out to
   Telegram for urgent ones.
5. **Room to grow without rework.** The features in "Planned features" are not
   built now, but the hooks they need (event journal, command API, settings
   layer, virtual sources, request counters, HTTP route registration) are part
   of the first stages because retrofitting them is expensive.

## Non-goals (for now)

- Device control beyond the planned automation hook. The dashboard is
  read-only in the first stages; the Aqara button is the only physical input.
- A rich web UI. The back office is server-rendered pages, no JS framework.
- Plugins as shared objects. Go's plugin package is not worth it; registration
  is at compile time.

## Package layout

```
cmd/domovoi/              main only: flags, signal context, application.Run — ~30 lines
internal/container        DI: Lazy[T] singletons, Registry[T], the Container struct, the Module contract + Passive
internal/application      the module list, application/modules/* glue, Run
internal/model            Reading, Value, Device, Metric, Event; metric catalogue + event transitions
internal/source           Source, Watcher, Controller, Sink, Batch; Poller (adapts a Source to the Sink)
internal/source/tuya          tuyacloud adapter
internal/source/aqara         aqaramcp adapter
internal/source/weather       Open-Meteo virtual source: the outside tile
internal/source/mqtt          Zigbee2MQTT adapter (later)
internal/source/system        Pi health + request counters (later)
internal/state            State: latest reading per device+metric, change → event classification
internal/storage/sqlite   infrastructure only: Open (PRAGMAs), goose migrations, migrations/*.sql
internal/storage/devices  Repository: Upsert, All, Prune
internal/storage/readings Repository: Write, Series, Extremes, Last, Before, LastAll, Prune; Point
internal/storage/events   Repository: Write, Since, Prune
internal/storage/store    the unit of work: Persist (one transaction per batch), Restore, Prune, and History for the scenes
internal/storage/alerts   Repository: Open, Close, Ack, Active
internal/storage/settings Repository: Get, Set, All
internal/alert            rules → Alert set (active/resolved), sinks
internal/notify/telegram
internal/scene            widgets, layout, Theme → *image.Paletted for a Surface
internal/icon             icon font (MDI subset) + typed icon names, weather/battery/kind tables
internal/display          Display interface, Surface, Frame
internal/display/epd7in5b     waveshare-epd7in5b-v2 backend
internal/display/pngfile      writes frame.png (dev preview, also on the Pi)
internal/display/httpview     serves the latest frame (a "browser display")
internal/config           YAML + ${NAME} secrets, Secret type, validation; runtime Settings overlay
internal/core             orchestrator: owns State, runs pollers, persists (write policy), Commands
internal/render           per-display coordinator: tick, diff, refresh Policy, Show
internal/i18n             go-i18n bundles (en, ru): T with named placeholders, N (CLDR plurals), number/date formatting
internal/httpd            the HTTP server: route registration, auth, back-office pages
internal/metrics          the one Prometheus implementation of every package's Metrics interface
internal/advisor          Advisor interface + context builder (LLM client later)
```

Everything under `internal/` because the binary is the product; if a piece
proves generally useful it graduates into its own repo like the clients did.

Dependencies point one way: `model` imports nothing; `source`, `display`,
`scene`, `storage/*`, `alert` import `model` and not each other — none of
them imports `config`, they take plain `Config` structs in constructors; adapters import their interface and their
vendor library; `core` is the only package that knows them all; `httpd`,
Telegram and the button reach `core` only through the `Commands` interface.
**Domain packages know nothing about the container or modules**: the glue
that puts a Tuya constructor into a registry or a repository behind a
singleton lives in `application/modules`, one file per module. `container`
imports the domain packages it holds; `application` imports `container`;
`cmd` imports `application` only.

## Assembly (`internal/application`)

`cmd/domovoi/main.go` parses flags, builds a signal-aware context and calls
`application.Run(ctx, Options{ConfigPath, Once})`. It imports no adapter. A
second binary (say, a PC preview tool) is another `main` with other
`Options`, not another wiring.

### Modules

Two levels that must not be confused: **what is compiled in** (modules) and
**what is enabled** (instances from the YAML). A module registers the kinds it
provides; the application creates instances from the config.

```go
type Module interface {
    Name() string
    Register(c *Container) error     // assembly phase: registries, routes; start nothing
    Start(ctx context.Context) error // non-blocking: spawn goroutines bound to ctx
    Stop(ctx context.Context) error  // wait for them; ctx carries the deadline
}

// Passive is embedded by modules with no background work.
type Passive struct{}

func (Passive) Start(context.Context) error { return nil }
func (Passive) Stop(context.Context) error  { return nil }
```

```go
// modules.go — the list; each entry is a small type in application/modules.
func modules() []Module {
    return []Module{
        modules.Tuya{},     // Register: c.Sources.Add("tuya", tuya.New)
        modules.Aqara{},
        modules.PNGFile{},  // Register: c.Displays.Add("pngfile", pngfile.New)
        modules.EPD7in5b{},
        modules.Scenes{},   // Register: c.Scenes.Add("overview", …)
        modules.Storage{},  // Register: c.DB builds via sqlite.Open, repositories, closer
        modules.Core{},
        modules.HTTPD{},    // Register: routes; Start: listen; Stop: shutdown
        modules.Telegram{},
    }
}
```

The glue types live in `internal/application/modules`, one file each
(`tuya.go`, `storage.go`, `httpd.go`, …): the only place where a domain
package meets the framework. The domain packages themselves (`source/tuya`,
`storage/readings`, `httpd`) never import `application` or `container`.
Adding a source is a new domain package plus one glue file (YAML section →
the adapter's `Config`) plus one line in the list; building without Telegram is deleting one line. The list order is the lifecycle order:
`Start` runs top-down, `Stop` bottom-up, so the store is up before the core
and the core before the HTTP server, and they go down in reverse.

Semantics: `Start` returns quickly; an error *at* start (port in use) comes
back synchronously. A fatal failure *after* start is reported with
`c.Fail(err)`, which cancels the root context with that cause; everything
stops and `Run` returns the cause. `Stop` gets a context with a deadline
(20 s plus the slowest panel's full refresh, so a frame in flight can finish)
and must return by it.

**Whoever creates, closes.** There is no separate list of closers: a module
that provides a singleton (the database) or builds instances from config
(the panels) keeps the references and releases them in its own `Stop`.
Bottom-up order guarantees the users are gone first: the core and the
render coordinators stop before the storage module closes the connection
and before the display module puts the panels to sleep. Modules that hold
what they created are pointers in the list (`&modules.Storage{}`). One
shutdown mechanism.

**Modules may wire, not only register.** A module that implements
`container.Assembler` gets `Assemble(c, cfg)` after every module has
registered and after the application wired its sources and displays, but
before `Start`: it sees the full registries and the built core, so alerts,
storage restore or a Telegram bot hook into the core from their own file
instead of growing the application's wiring.

### Container (`internal/container`): lazy singletons

Its own package so that `application` and tests share it. No DI framework:
`fx`/`dig` are reflection and magic, `wire` is another tool. A 40-line
generic covers it:

```go
type Lazy[T any] struct {
    mu    sync.Mutex
    build func() (T, error)
    built bool
    v     T
    err   error
}

func (l *Lazy[T]) Provide(build func() (T, error)) // in Register, by the module that owns the singleton
func (l *Lazy[T]) Get() (T, error)                  // builds once; "not provided" if nobody called Provide
func (l *Lazy[T]) Set(v T)                          // tests install a fake; panics if already built
```

```go
// The container is the manifest of singletons. Adding a singleton edits this
// struct — deliberately, so the wiring stays visible in one place. Sources,
// displays and scenes are instances, not singletons: they go through the
// registries.
type Container struct {
    Config   Lazy[*config.Config]
    Logger   Lazy[*slog.Logger]
    Metrics  Lazy[*metrics.Registry]
    DB       Lazy[*sql.DB]       // provided by modules.Storage via sqlite.Open
    Store    Lazy[*store.Store]  // the unit of work over the repositories
    State    Lazy[*state.State]
    Core     Lazy[*core.Core]    // also the Commands implementation
    HTTP     Lazy[*httpd.Server]

    Sources  Registry[source.Constructor]  // kind → constructor; Add twice = error
    Displays Registry[display.Constructor]
    Scenes   Registry[scene.Scene]

    Check bool // -check: build everything, touch nothing outside the process
}
```

A singleton is built on first `Get()` and reaches its neighbours the same
way (`DB` reads the path from `c.Config.Get()`, `Store` takes `c.DB`,
`Core` takes `c.State` and `c.Store`). What nobody asks for is never
created: `-once` with no Telegram section never builds it. A `Get` from
inside a builder that leads back to the same `Lazy` fails with `ErrCycle`
instead of deadlocking. Tests call `c.Store.Set(fake)` before the first
`Get` and assemble the whole application on fakes.

### Run

```
load config → new container → Register every module → instances from config
(unknown kind = error listing the known ones) → Start top-down
→ wait for ctx (signal or Fail) → Stop bottom-up with a deadline
→ return the cause
```

## Canonical model (`internal/model`)

```go
// DeviceID is "<source name>:<native id>", e.g. "tuya:bf22826zxhnglohk".
// The prefix is the *name* of the configured source section (defaulting to
// its kind), not the kind: two Tuya accounts or two MQTT brokers are two
// names. A name is a key into history and must never be renamed once used.
type DeviceID string

type Device struct {
    ID   DeviceID // its prefix names the source; no separate Source field
    Name string   // as named in the vendor app ("Спальня")
    Room string   // vendor room when known
    Kind Kind     // ClimateSensor, LeakSensor, Lock, Outlet, Button, Thermostat, Hub, Virtual, Other
}

// Metric is a stable, vendor-neutral name. Sources map into this set.
type Metric string

const (
    Temperature      Metric = "temperature"       // °C
    Humidity         Metric = "humidity"          // %
    Battery          Metric = "battery"           // %
    Leak             Metric = "leak"              // bool
    Locked           Metric = "locked"            // bool
    Power            Metric = "power"             // bool (outlet on/off, thermostat switch)
    TargetTemperature Metric = "target_temperature" // °C, thermostat setpoint
    Heating          Metric = "heating"           // bool, thermostat valve open
    PowerDraw        Metric = "power_draw"        // W, metering outlets
    Online           Metric = "online"            // bool, emitted by every source for every device
    TemperatureAlarm Metric = "temperature_alarm" // text: vendor alarm state ("upperalarm", "")
    HumidityAlarm    Metric = "humidity_alarm"    // text
    Note             Metric = "note"              // text: free-form (advisor, weather summary)
)
// Weather (stage 2, device weather:home): outdoor temperature and humidity
// are the same Temperature/Humidity metrics — sparklines and extremes come
// for free — plus WindSpeed (m/s), PrecipitationProbability (%),
// WeatherCode (text: WMO code, worded through i18n), ForecastHigh,
// ForecastLow (°C), Sunrise, Sunset (text HH:MM). Later, with the Zigbee
// stick: PowerDraw (W), Energy (kWh, monotonic counter); derived DewPoint,
// AbsoluteHumidity; Present, CPUTemperature, RequestsToday.

type ValueKind int // Number, Bool, Text

type Value struct {
    Kind ValueKind
    Num  float64 // Number, and Bool as 0/1
    Text string  // Text
}

type Reading struct {
    Device DeviceID
    Metric Metric    // the unit comes from the metric catalogue, not from the reading
    Value  Value
    At     time.Time // when the source observed it (poll time if unknown)
}

// Event is a transition worth remembering, as opposed to a reading, which is
// a level: the door was unlocked, a leak started, a device went offline, the
// button was pressed, an alert was raised.
type Event struct {
    Device  DeviceID
    Kind    EventKind // Unlocked, Locked, LeakStarted, LeakEnded, Offline, Online,
                      // ButtonPressed, AlertRaised, AlertResolved, …
    Detail  string    // "double" for a button, the rule name for an alert
    At      time.Time
}
```

`Kind` is a hint (icon, rule targeting), not what a scene dispatches on:
widgets are chosen by the metrics a device actually has (temperature +
humidity → a climate tile), so a new vendor's thermometer renders without
touching a scene.

`model` also holds the **metric catalogue** — for every metric its value
kind, unit and the **event transitions** it produces (`locked` true→false =
`Unlocked`, `leak` false→true = `LeakStarted`, `online` true→false =
`Offline`) — so a metric and its events are described in one table, and the
classifier in `state` is driven by it rather than by its own switch.

Three value kinds, no generics, no interface{}. Storage maps 1:1 onto
`num REAL, text TEXT`. Unknown vendor points are dropped by the adapter (logged
once at debug level), not smuggled through as opaque metrics — a metric only
exists once something renders or alerts on it.

## Sources (`internal/source`)

```go
// Source is a polled provider: one Poll is one pass. Batch.Devices carries
// the device list whenever the source has it — on the first pass always,
// later whenever it changed or whenever the vendor call returns it anyway
// (Tuya's listing and Aqara's status table both do); nil means "unchanged".
// No separate discovery method, no periodic rediscovery loop.
type Source interface {
    Name() string
    Poll(ctx context.Context) (Batch, error)
}

type Batch struct {
    Devices  []model.Device // nil = unchanged since the previous batch
    Readings []model.Reading
}

// Watcher is a push provider (MQTT). It delivers into the same Sink the
// Poller uses, so the core has exactly one intake and tests one fake.
type Watcher interface {
    Watch(ctx context.Context, sink Sink) error
}

// Sink is the core's intake. A source failure is reported as Health, never
// as fabricated readings: the lock did not go offline, the cloud did.
type Sink interface {
    Batch(source string, b Batch)
    Event(e model.Event)
    Health(source string, err error) // nil = healthy again
}

// Controller is the optional write side, for the automation hook: sources
// that can act implement it; the core refuses actions on those that do not.
type Controller interface {
    Set(ctx context.Context, device model.DeviceID, metric model.Metric, v model.Value) error
}
```

`Poller` adapts a `Source` to the `Sink`: one goroutine per source, `Poll`
every `interval`, `sink.Batch` on success, `sink.Health(err)` on failure
with backoff (1 min → 15 min) and `sink.Health(nil)` on recovery. The
vendor's own per-device online flag stays an ordinary `online` reading;
source health is separate state and never becomes readings, events or
history. Upstream request counts go through the adapter's own `Metrics`
interface (`IncRequest(source)`), the same way as every other counter — no
separate meter mechanism.

A source's configured `name` is the prefix of its device IDs: the adapter
receives it in its `Config` and builds IDs with `model.NewDeviceID(name,
native)`. Names must not contain `:` (validated by config).

**Virtual sources are ordinary sources.** A weather source owns the virtual
device `weather:home`; a derived-metrics source reads the State (it gets a
`state.Reader` at construction) and emits dew point and "ventilate" hints; a
presence source pings phones; a `system` source reports the Pi's CPU
temperature, disk space and request counters. None of them needs anything
the core does not already have.

### Registration and configuration of a source

Domain packages do not import `config`. An adapter declares its own plain
`Config` struct and takes it in the constructor; the glue in
`application/modules` knows the YAML shape, decodes the section strictly
(unknown keys are errors — a typo in a credential name must not be silent)
and maps it, revealing `Secret`s into plain strings only there:

```go
// internal/source/tuya
type Config struct {
    Name      string // source name, prefix of its device IDs
    AccessID  string
    AccessKey string
    Region    tuyacloud.Region
}
func New(cfg Config, log *slog.Logger, m Metrics) (*Source, error)

// internal/application/modules/tuya.go
type tuyaSection struct {
    AccessID  config.Secret `yaml:"access_id"`
    AccessKey config.Secret `yaml:"access_key"`
    Region    string        `yaml:"region"`
}
// name, kind and interval are common to every source section and are read
// by the application, not by the adapter.
```

The parallel struct costs a few lines per adapter and buys: domain tests
built from a literal, a config format that can change without touching the
domain, and `config` known only to `application/modules` and `container`.

```yaml
sources:
  - kind: tuya
    name: tuya          # optional, defaults to kind; prefix of every DeviceID; immutable once used
    interval: 60s
    access_id: ${TUYA_ACCESS_ID}
    access_key: ${TUYA_ACCESS_KEY}
    region: eu
  - kind: aqara
    interval: 90s
    username: ${AQARA_USERNAME}         # the account the source signs in with…
    password_md5: ${AQARA_PASSWORD_MD5} # …the service only ever sees the MD5
    region: RU                          # EU, RU, US, CN, KR or SG
    # api_key: ${AQARA_MCP_KEY}         # a key from the login page instead of, or until it expires before, signing in
```

### Adapters

**tuya** — every poll reads `DevicesStatus` in batches of 20 (the client
batches); every `ListEvery`-th poll (default 5) also runs the device
listing (names, `Online`, `update_time`; rooms are absent) and fills
`Batch.Devices`. One request per minute plus one every five minutes,
instead of two per minute. Readings are stamped with the device's
`update_time` from the listing (clamped to the poll time), not with the
poll time, so the cloud's cached values of a silent sensor carry their real
age. Tuya's `update_time` moves on network events (online, offline, a
battery swap), not on every report, so the State re-stamps a *changed*
value with the batch time unless the vendor stamp is newer than the last
moment the previous value was observed: a device's newest reading time is
then its last sign of life — last change or last network event — and two
different values never share a moment in history. After a restart with an
empty database there is no previous value to compare against, so a quiet
device's first frame may say "no data since" its last network event until
its value moves. A device without a sign of life for `state.DeviceStaleAfter`
(3 h) is stale even on a healthy source, and its tile says "no data since
…" where a live tile shows its extremes. The cloud's `online` flag is not
trusted alone — it stayed true for a sensor a day silent. Per-point
timestamps exist in Tuya's `shadow/properties` endpoint (one request per
device); if false "no data" notes ever appear on quiet rooms, fetching it
on listing polls is the next step. Offline devices
are read too, so a tile keeps its last known values next to the offline
mark. Mapping by
data-point code, scale from the product `Specification`
fetched once per product and cached (a failed fetch is held off for ten
minutes and warned once; numbers use the points table's default scales
meanwhile, so a sensor tile never goes blank over a flaky endpoint):
`va_temperature→Temperature`, `va_humidity→Humidity`, `battery_percentage→Battery`,
`temp_alarm→TemperatureAlarm`, `hum_alarm→HumidityAlarm`, `switch*→Power`,
`temp_set→TargetTemperature`, `valve_state` (`open`)→`Heating`,
`cur_power→PowerDraw` (shown next to the outlet's state in the hallway
strip; `add_ele` energy increments wait for storage). The strip is always
one row: with many devices its cells and its height shrink together, down
to 55 % of the design size, so the climate tiles keep their space. A thermostat
keeps its climate tile — setpoint and heating state on the detail line —
and stays out of the hallway strip.
Device kind by category (`wsdcg`→ClimateSensor, `cz`/`pc`→Outlet,
`wk`→Thermostat, else Other).

**aqara** — one `Statuses(StatusFilter{})` call per poll, which also returns
names, types and rooms, so it fills `Batch.Devices` every time; kind from
`Device.Type`; `lock_state "1"→Locked true`, `water_leak→Leak`,
`on_off→Power`, `online_offline→Online`; any `lock_state` other than `"1"`
reads as unlocked, and every distinct value is logged once at Info so the
unlocked value can be confirmed from the journal. **Known gaps, stated openly:** no
power metering, no clicks and **no battery level** for any Aqara device
through MCP (low-battery alerts cover Tuya sensors only until the Zigbee
stick); the `lock_state` value of an *unlocked* U200 has not been observed
yet — verification item for stage 2 (unlock, poll, record). Implements
`Controller` later through `device_status_control` for the automation hook.
**The key renews itself.** The MCP key the login page hands out expires
after days (no documented lifetime; observed under two weeks) and there is
no refresh token — the page simply signs in again, with one plain POST of
the account, the MD5 of the password and the region (`RU` for a Kazakh
account). `aqaramcp.WithLogin` does the same inside the client: a rejected
key is replaced within the poll that met the rejection and the call is made
again, so the screen never sees the gap; a refused sign-in (wrong account,
password or region) is a source error like any other and is not retried
for an hour. Every sign-in is logged at Info with the region and the key's
length — never the key — which is also how the real lifetime gets measured.
A dedicated Aqara account, shared into the home as a family member, keeps
the owner's password off the Pi; its MD5 is what the credential store holds
(the client takes only the digest, never the password). Verified live: the
member account's key reads every device, and a new sign-in does not revoke
earlier keys. **Endpoint IDs are per account**: the same devices carry
different `Aqr~…` IDs under the owner's key and under the member's, so
switching accounts changes every `aqara:` device ID — the listing rule in
the State section retires the old ones.

**weather** (stage 2) — a virtual source on Open-Meteo (free, no key, JSON):
one request every `interval` (default 20 min) for current conditions plus
today's daily forecast and sun times at the configured `latitude`/
`longitude`, in the configured timezone (or `auto`, the coordinates' own
zone, when none is configured — the system zone has no name the API would
take); readings are dated with the UTC offset the answer reports, so the
stamps are right whatever zone the process runs in. One device
`weather:home` of kind `Virtual`; `online` reflects the last fetch. The first source that is not a
vendor cloud, which is exactly why it comes early: it proves the virtual
source path.

**mqtt** (stage 5) — subscribes `zigbee2mqtt/#`, maps Z2M exposes to metrics,
emits `Online` from availability topics and **button clicks as events**.

**advisor** (later) — see "Advisor" below; plugs in through the same table.

## State and events (`internal/state`)

The single owner of "what is true now": `map[DeviceID]map[Metric]Reading`,
devices, and **per-source health** (`LastOK`, `LastError`, `Err`). Fed only
by the core goroutine; readers get snapshots (`Snapshot()` returns an
immutable copy). Reports changes as `Change{Device, Metric, Old, New}` so
the core can persist only changes and the renderer can decide whether
anything visible moved. `Snapshot.Stale(device)` derives from the health of
the device's source (no successful batch for longer than `stale_after`):
scenes show "Tuya: no data since 12:03" and dim the tiles instead of
pretending the devices went offline. At start the State is seeded from the
last stored point of every series, so the first frame after a reboot is not
empty.

A **classifier** turns changes into events using the transition table of
the metric catalogue in `model`. Events from watchers (button) pass through as they are. Events go
to the store, to the alert engine, to the advisor context and to the back
office timeline — the journal is what makes "what happened today" answerable.

**A listing defines the set.** A source that fills `Batch.Devices` hands
over its whole inventory (Tuya on its first poll and every fifth, Aqara and
weather on every poll), so devices of that source missing from it are
*retired*: dropped from the state with their readings, logged by the core,
gone from the next frame. History stays in storage. That is how a device
removed from the account, a sensor re-paired under a new ID, or the ghosts
of a different cloud account (Aqara's endpoint IDs are per account) leave
the screen without anyone editing a database. An empty listing is
distrusted and retires no one: a cloud answering nothing is far likelier
than a home with nothing in it. `Restore` seeds what the database knows
about the *configured* sources — a renamed or removed section's devices
stay in the file, not on the screen — and the first listing trims the rest
before the first frame.

## Storage (`internal/storage`)

`modernc.org/sqlite` through `database/sql`, WAL, `synchronous=NORMAL`.
`storage/sqlite` is infrastructure only — `Open(path)` applies the PRAGMAs
and runs the migrations and is called by the storage module when it builds
the `DB` singleton (under `-check` with `sqlite.Memory` instead of the
configured file, after verifying the file's directory exists). Everything else is one repository package per entity
(`devices`, `readings`, `events`, `alerts`, `settings`), each a `Repository`
constructed with a ready connection, holding the SQL of its entity and
nothing else — no opening, no closing, no policy:

```go
package readings

// DBTX is what *sql.DB and *sql.Tx have in common, so a repository can join a
// transaction spanning entities (raise an alert and journal the event).
type DBTX interface {
    ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
    QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func New(db DBTX) *Repository
func (r *Repository) Write(ctx context.Context, rs []model.Reading) error   // writes what it is given
func (r *Repository) Series(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) ([]Point, error)
func (r *Repository) Extremes(ctx context.Context, id model.DeviceID, m model.Metric, since time.Time) (lo, hi Point, ok bool, err error)
func (r *Repository) Last(ctx context.Context, id model.DeviceID, m model.Metric) (Point, bool, error)
func (r *Repository) Before(ctx context.Context, id model.DeviceID, m model.Metric, t time.Time) (Point, bool, error)
func (r *Repository) LastAll(ctx context.Context) ([]model.Reading, error)   // restore after a restart
func (r *Repository) Prune(ctx context.Context, before time.Time) (int64, error)
```

Repositories keep no state: series IDs are resolved per write (two cheap
statements per series per batch), which keeps a repository over a
transaction free — the `device+metric → series.id` cache the first draft
planned is unneeded at this scale. `storage/store` is the unit of work
above them: it opens the one transaction per batch (`Persist` upserts
devices, writes readings and journals events together), runs the restore
query and the pruning, and answers the history queries scenes need. The
core sees it as its `Store` interface, the render coordinator as
`History`; neither touches SQL.

```sql
CREATE TABLE devices  (id TEXT PRIMARY KEY, source TEXT NOT NULL, name TEXT NOT NULL, room TEXT,
                       kind TEXT NOT NULL, first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL);
CREATE TABLE series   (id INTEGER PRIMARY KEY, device TEXT NOT NULL REFERENCES devices(id),
                       metric TEXT NOT NULL, unit TEXT, UNIQUE (device, metric));
CREATE TABLE readings (series INTEGER NOT NULL REFERENCES series(id), at INTEGER NOT NULL,
                       num REAL, text TEXT, PRIMARY KEY (series, at)) WITHOUT ROWID;
CREATE TABLE events   (id INTEGER PRIMARY KEY, at INTEGER NOT NULL, device TEXT, kind TEXT NOT NULL, detail TEXT);
CREATE INDEX events_at ON events (at);
CREATE TABLE alerts   (id INTEGER PRIMARY KEY, rule TEXT NOT NULL, device TEXT, severity TEXT NOT NULL,
                       started_at INTEGER NOT NULL, ended_at INTEGER, acked_at INTEGER, message TEXT);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL);
```

The schema knows no vendor and no metric: a new source, real or virtual,
is new rows, never new columns. `series` gives the hot table an integer key
instead of a 45-character Aqara ID per row (a fourfold saving). Time is
unix seconds — minute-cadence data needs no milliseconds. A Number or Bool
lands in `num`, a Text in `text`.

**Write policy lives in the core (`persist.go`), not in the repositories.** `State.Apply`
already reports which readings changed; the core writes those, plus a
`heartbeat` row (default 1 h) for series that have not changed — charts
carry the last value forward, and "still alive" is what the `online` metric
is for. The core keeps its own "last written at" map for the heartbeat. One
transaction per poll batch, bounded by a 10 s timeout; a failed write is
logged and counted (`domovoi_store_errors_total`), never fatal, and the
series it carried are not marked as written, so the next batch writes their
values again. Readings are stored at their own stamp; a heartbeat row
carries the value forward to the poll time, and the State stamps a changed
value with its batch time whenever the vendor stamp is not newer than the
last observation of the previous value, so a change always lands after the
heartbeat that preceded it.
Prune older than `retention` (default 90 d) once a day from the core's
housekeeping goroutine, in one transaction: readings and events first, then
the series they left empty, then the devices unseen for longer than the
retention with no series left — a retired device's row goes when its
history does; freed pages are reused, no VACUUM. Because only
changes are written, the last row of a series *is* the last change
("locked 2 h ago" is one query, `Last`). Heartbeat rows are stamped with
the poll time, so a chart carries a value forward; changed readings keep
the stamp the State gave them.

**Sizing** for this home (~40 series, climate values changing 10–15×/h):
~3 500 rows/day at ~25 bytes → a plateau around 10 MB at 90 days; events
and alerts are noise next to it.

**SD card wear is not a design constraint.** Write traffic is ~30–40 MB/day
(one transaction per poll batch is what counts, and that number is the same
whether values are deduplicated or not). Even with a 20× write
amplification that is ~300 GB/year against ~10 TB of endurance for the
entry-level card in use (Kingston Canvas Select Plus, TLC, a few hundred
P/E cycles): decades. Deduplication stays for data quality and query
speed, not for the card. No buffering, no deferred flushes: every batch
commits immediately, `synchronous=NORMAL` under WAL loses at most the last
transaction on power loss and never corrupts the file. The real risks are
power loss and silent card death, so: a nightly `VACUUM INTO` backup to the
mini-PC (one consistent file, ten lines behind a back-office button), a
journald size limit with logs at `Info`, no swap on the Pi.

**Queries** the scenes need: latest N points of a series (sparkline),
min/max since a moment (daily extremes in red), last row (last change),
events since a moment. Monotonic counters (energy) get a daily-delta query
when they appear.

**Later:** `readings_hourly (series, hour, min, max, avg, n)` filled once a
day and kept forever — ~10 MB/year, enables "this time last year" and trend
material for the advisor while raw data still expires at 90 days. Frame
archives (timelapse) go to the filesystem, not the database.

### Migrations: goose

`github.com/pressly/goose/v3` with the Provider API
(`goose.NewProvider(goose.DialectSQLite3, db, fsys)`, `Up(ctx)` at `Open`),
SQL files under `internal/storage/sqlite/migrations/NNNN_name.sql` via
`go:embed` — one ordered sequence for the whole schema, never per entity —
logger adapted to slog. Measured cost: +1.3 MB of binary and four small
linked modules — the many drivers in goose's own go.mod are for its CLI and
are pruned from ours. Conventions: forward-only (no `Down` sections; rolling
back is restoring the database file), one transaction per file (SQLite DDL
is transactional), refuse to start when the database version is newer than
the binary knows. Go migrations exist if a data transformation ever needs
code. Schema changes are expected a few times a year, never per source.

### Three habits that keep Postgres a one-package change

Everything SQLite-specific lives in `internal/storage`; the rest of the
program talks to it through small interfaces (`History`, `Journal`,
`Settings`). A future `storage/postgres` is a second infrastructure package
plus its own migration directory and repository variants where the SQL
differs (goose supports both dialects), with nothing else
touched — provided the queries are written this way from the start:

1. `INSERT … ON CONFLICT DO UPDATE`, never SQLite's `INSERT OR REPLACE`.
2. Dates computed in Go, never in SQL: "local midnight" is a number passed
   in, not a `strftime` call.
3. `?` placeholders, with a ten-line rebind helper if `$1` is ever needed.

`WITHOUT ROWID` and `INTEGER PRIMARY KEY` stay in the migration files, where
dialects are allowed to differ. Honest note: on the Pi, Postgres is not a
goal; if data is wanted on the mini-PC, CSV export or `/metrics` scraping is
cheaper than moving the store.

## Configuration: file + runtime settings (`internal/config`)

Two layers:

1. **The file** (`/etc/domovoi.yaml`): structure and secrets references —
   which sources and displays exist, credentials as `${NAME}` placeholders,
   listen addresses. The YAML itself never contains a key and can be shown
   or committed.
2. **Settings** (the `settings` table): everything a person may want to tune
   at runtime from the back office — poll intervals, alert thresholds and
   rules, scene assignment and rotation, quiet hours, Telegram severity,
   **device overrides** (display name, room, hidden, order — vendor app
   names like "Комната по умолчанию" are defaults, not destiny), **button
   gestures → commands** (single → next page, double → refresh, long →
   acknowledge + mute are defaults in the table, not code), and the
   language. The file provides defaults; a settings row overrides the
   matching key. Secrets are never settings.

The file also carries `timezone` (IANA name, default the system zone): the
`*time.Location` is passed explicitly to whatever computes "local midnight"
or quiet hours, never read from the environment inside domain code.

**Reload** (stage 6, with the back office; the shape is fixed now so
nothing has to be retrofitted). Two mechanisms for two kinds of parameter:

1. *Structural* parameters (credentials, region, which sources and displays
   exist): an instance is immutable. `Reload` re-reads the file, hashes
   each section against the previous one and, for the changed ones, builds
   a new instance through the same constructor and asks the core to swap
   it — stop the old poller, start the new. Adapters are stateless, so
   rebuilding is free (Tuya loses its cached token: one extra request).
2. *Hot* parameters (intervals, thresholds and rules, scene assignment and
   rotation, quiet hours, Telegram severity) owned by the settings layer:
   their owners expose explicit `Update(...)` methods under a mutex, called
   by the core when a setting changes. No rebuild, no knowledge of config.

What this needs from day one: `Config` values with no reference to anything
global, constructors without side effects, and a core operation "replace
the source named X". SIGHUP triggers the same reload.

## Secrets

Four of them: Tuya access ID and key, the Aqara account and the MD5 of its
password (a dedicated family-member account, not the owner's), the Telegram
bot token, the back-office password. The database holds none, ever.

- **At rest on the Pi: files in a private directory**, not environment
  variables. Under systemd that is `LoadCredential=`: files under
  `/etc/credstore/` (`0600 root`) handed to the service in a private
  `$CREDENTIALS_DIRECTORY` for its lifetime — invisible in
  `/proc/<pid>/environ`, not inherited by children, absent from core dumps,
  optionally encrypted with `systemd-creds encrypt` under the host key.
  Under Docker Compose it is file-based `secrets:` mounted read-only at
  `/run/secrets`. The application resolves `${NAME}` from
  `$CREDENTIALS_DIRECTORY`, then `/run/secrets`, then the environment, so
  the same config runs under either supervisor and on the PC. The Pi has no TPM and the host key lives on the same SD card, so this
  guards against casual reading, not against theft of the card — an accepted
  risk: every secret is revocable, and the README lists what to reissue.
- **Not root.** A dedicated `domovoi` user in the `spi` and `gpio` groups;
  the unit sets `NoNewPrivileges`, `ProtectSystem=strict` and a single
  writable state directory for the database.
- **In the config.** `${NAME}` resolves first to the file
  `$CREDENTIALS_DIRECTORY/NAME`, then `/run/secrets/NAME`, then to the
  environment variable — the same YAML runs under systemd or Docker on the
  Pi and with `. .env && go run` on the PC. The repository ships
  `domovoi.env.example` with empty values.
- **Delivery.** Over SSH from the user's PC, by the user, permissions set
  before content is written:
  `ssh pizero.local 'sudo install -m 600 /dev/stdin /etc/credstore/tuya_access_key' < tuya_access_key`.
  Keys never pass through chat, git or shell history.
- **In the process.** `config.Secret` implements `String`, `LogValue` and
  `MarshalYAML` as `***`, so a `%+v` of the config, a debug log or a
  settings page cannot leak a key. The client libraries already truncate
  error bodies and never print headers.
- **Back office.** Secrets live in the file layer, so the settings page
  neither knows nor edits them. Basic auth over the LAN address only; TLS is
  a one-option addition if ever wanted. Inbound Telegram commands are
  accepted only from the configured chat ID.
- **Cloud tokens.** Tuya's per-session access token and the Aqara MCP key
  both live in the client's memory: the Aqara key is minted by signing in
  at start and re-minted when the service rejects it. Nothing token-like is
  written to disk; a restart costs one sign-in.

## Logging

`log/slog`, no third-party logger. One `*slog.Logger` singleton in the
container; each module gets a child with `component` set
(`log.With("component", "source.tuya")`). `TextHandler` to stderr — journald
adds time and keeps it, `journalctl -u domovoi -f` is readable; `JSONHandler`
by config if logs ever go to a collector. Levels: `Error` needs a human
(source failed to construct, database unopenable); `Warn` survivable but
notable (poll failed and backed off, panel error, missing specification);
`Info` milestones (module start/stop, frame shown with mode and duration,
alert raised/resolved, journal event); `Debug` per-poll counts, policy
decisions, dropped data points (once per code). Default `Info`.

Library code returns wrapped errors and never logs; the decision-makers log
(Poller, core, render coordinator, `Run`), so one error appears once at its
level. The back-office log page is a ring buffer of the last N records,
implemented as a second `slog.Handler` behind a small fan-out; journald
stays the source of truth. Keys, tokens and credential-bearing bodies never
reach a log line.

## Alerts (`internal/alert`)

Small typed rules, no expression language:

```yaml
alerts:
  - name: leak
    metric: leak
    when: is_true
    severity: urgent
    action: {device: "aqara:Aqr~…", metric: power, value: false}   # automation hook, later
  - name: door unlocked
    metric: locked
    when: is_false
    for: 10m
    severity: warning
  - name: humidity
    metric: humidity_alarm
    when: equals
    value: upperalarm
    severity: warning
  - name: offline
    metric: online
    when: is_false
    for: 15m
    severity: info
  - name: low battery
    metric: battery
    when: below
    value: 15
    severity: info
```

Conditions: `is_true`, `is_false`, `equals`, `below`, `above` — numbers
included from day one, because the first real rule after leaks is a battery
threshold.

`device:` optionally narrows a rule; `for:` requires the condition to hold that
long before firing (hysteresis for flapping — the `online` metric of a
Bluetooth sensor can toggle every poll, so the offline rule carries `for:`
and the Telegram sink gets a hold-down of its own). Compound conditions ("leak
AND the outlet is on", "unlocked at night") will arrive as an `all:` list of
conditions inside a rule, still typed — never as an expression language. Evaluated on every change and on
a 1-min tick (for `for:`). Output is the alert set with transitions
raised/resolved/acknowledged — each also an event. Sinks: `display` (alerts are
part of the render snapshot), `telegram` (severity ≥ configured; raise and
resolve messages; during quiet hours only `urgent` is sent immediately,
lower severities are held for the morning digest), `log`. `action` is the automation hook: when present and
the target source implements `Controller`, the core executes it on raise.
Sources' own alarm points (Tuya `temp_alarm`) come through as ordinary
readings and are matched with `equals` — no duplicate threshold logic.

## Commands (`internal/core`)

One command API, several input adapters:

```go
type Commands interface {
    NextPage(display string)
    Refresh(display string)        // force a full refresh
    Acknowledge(alertID int64)
    AcknowledgeAll()               // the button's long press
    Mute(d time.Duration)          // silence Telegram
    Reload()
}
```

The Aqara button (single → next page, double → refresh, long → acknowledge
all + mute 1 h), the back office and Telegram bot commands all call this and
nothing else. New inputs never touch the core.

## Display (`internal/display`)

```go
// Surface describes what a display can show. Scenes are laid out against it.
type Surface struct {
    Bounds  image.Rectangle
    Palette color.Palette // e.g. white, black, red; mono panels: white, black
    Partial bool          // supports windowed updates (mono only on our panel)
    FullRefresh time.Duration // for scheduling and logs
}

// Frame is a rendered scene in the display's palette.
type Frame struct {
    Image  *image.Paletted
    Window image.Rectangle // zero = whole frame
    Mode   Mode            // Full, Fast, Partial
}

type Display interface {
    Surface() Surface
    Show(ctx context.Context, f Frame) error
    Close(ctx context.Context) error // sleep / flush
}
```

The contract every backend keeps: `Show` blocks for the refresh and returns
promptly once `ctx` is cancelled, leaving the glass in an unknown state that
the coordinator forgets and redraws; a display serialises `Show` and `Close`
itself, because `Close` may arrive while a cancelled `Show` is still
unwinding and must wait for it before putting the panel to sleep. A `Show`
cut short by shutdown is not counted as a display error.

Backends: `epd7in5b` (wraps our driver; every `Show` is power-up → Init /
InitFast / InitPartial → transfer → Sleep, the panel never stays powered
between frames), `pngfile` (writes the frame, ignores modes), `httpview`
(keeps the last frame, serves `/frame.png` and a page that reloads it — also
what the back office and Telegram `/frame` show). Several displays run at
once, each with its own scenes and loop.

### Refresh policy (`internal/render`, display-agnostic)

**Start.** A coordinator draws its first frame on the first `Wake` or when
the start grace runs out, whichever comes first. The core wakes the displays
when the last configured source has answered at all — with data or with an
error, so a dead cloud cannot keep the starting frame up — and again when a
source that started with an error delivers its first data, so the real
picture does not wait for a tick. Nothing wakes while a source is still
pending: its answer will. The grace is at least one poll timeout plus five
seconds (30 s at minimum), so a slow cloud does not cost a "starting" frame
that the first answer would replace a moment later.

**Rendering is tick-driven, not change-driven.** Each coordinator renders
on a fixed tick (`tick`, default 5 min, aligned to the wall clock) and
otherwise only when an alert transition arrives — the single out-of-band
trigger. Readings arriving between ticks wait for the next one. This is what
makes the refresh budget hold: two sources polling at different phases, or a
clock widget, cannot produce two flickers a minute. The time in the header
is therefore a freshness stamp ("updated 12:05"), not a clock: it shows the
tick's time, never seconds. A real minute clock is possible with `tick: 1m`
— the policy then runs a monochrome partial update of the changed window
each minute (~2 s of local flicker, 1 440 times a day) and a full refresh
hourly; whether that feels acceptable is decided on the real panel in
stage 3, the default stays 5 min.

On each render the mode is decided from the diff between the frame on the
glass and the new frame:

- nothing changed → no update;
- red pixels changed, or the display has no partial mode, or the last full
  refresh is older than `full_every` (default 1 h), or more than `max_partials`
  (default 12) since the last full → **Full** (or **Fast** when the change is
  not an alert and fast mode is allowed);
- otherwise → **Partial** on the bounding box of changed pixels, aligned as
  the driver requires, and only if no red lies inside the window (red cannot
  be erased by a partial update — panel fact);
- never two fulls closer than `min_full_interval` (3 min) unless an urgent
  alert appeared — alerts pre-empt the cadence;
- a failed `Show` leaves the glass in an unknown state: the coordinator
  forgets the previous frame and the next update is **Full**;
- **quiet hours** (night): a longer tick, no fast refreshes, one full clear
  cycle at a configured hour to wash out ghosting.

The policy is pure — inputs are the two frames, the surface, the clock and
the counters — and unit-tested against synthetic frames; the panel timings
only tune its defaults.

## Scenes (`internal/scene`)

A scene is a tree of widgets laid out on a grid for a given `Surface`:

```go
type Scene interface {
    Render(ctx context.Context, s display.Surface, v View) (*image.Paletted, error)
}
```

`View` is the render snapshot: devices, latest readings, alerts, recent
events, history queries (a function the widget calls for sparklines), clock.
Widgets draw with **semantic colours** (`Paper`, `Ink`, `Accent`, `Muted`)
that a `Theme` maps to the palette — on a three-colour panel `Accent` is red,
on a mono panel it becomes black-on-inverted, on a future grey panel a grey.
Widgets: `Tile` (big value + label + trend arrow), `List` (device rows with
status glyphs), `Sparkline`, `Clock`, `AlertBar`, `Text` (advisor note,
weather line), `Timeline` (last events), `QR` (link to the back office).
Layout is a `Grid` with row/column weights and spans, so the same scene
definition fills 800×480 or 250×122.

Scenes are Go code in the first stages — the most hard-coded spot in the
design, accepted knowingly. Two rules keep the exit open: every widget is
bound to explicit (device, metric) pairs or to a metric-set query
("all devices with temperature"), never to "the first climate sensor"; and
a scene is data (a widget tree) that a Go constructor happens to build. A
YAML scene loader later is then a thin parser over the same widgets.

All on-screen text goes through `i18n` (see Localisation); no literal
labels in widgets.

### Screen 1: `overview` — the product spec

What the first frame shows, top to bottom, on 800×480:

1. **Alert bar** (only when something is active): severity icon (`alert`
   for urgent, `bell` otherwise), message, "since 12:03"; red on the colour
   panel. Absent otherwise — the space goes to the grid.
2. **Header**: date, the freshness stamp "updated HH:MM" (the tick time),
   and per-source freshness ("Tuya 12:05 · Aqara 12:04 · weather 11:45"); a
   stale source is shown in accent with `cloud-off` and "no data since …".
3. **Climate grid**, one tile per device that has `temperature`: the room
   icon (by Kind, overridable in settings: bed, sofa, desk, stove) and room
   name, temperature large (one decimal) with a thermometer glyph, humidity
   with a droplet, today's min/max in small type (the current value in
   accent when it *is* today's extreme), a trend arrow from the last hour, a
   battery glyph by level (accent below 20 %), dimmed when stale or offline.
   Five tiles today: **outside first** (the `weather:home` device: the
   condition icon large from the WMO-code table with the condition worded
   through i18n, temperature, humidity, precipitation probability, wind with
   its glyph, forecast high/low, sunrise/sunset with their glyphs), then
   кабинет, спальня, зал, кухня.
4. **Hallway strip**: lock (`lock`/`lock-open` with "since"), the washing
   machine outlet (`washing-machine` + `power-plug`/`power-plug-off`), the
   two leak sensors (`water-off` dry / `water` LEAK). Anything alarmed in
   accent.
5. **Footer**: the advisor's note when present, otherwise the last two
   journal events ("14:02 door unlocked", "13:40 kitchen sensor back online").

Everything is bound by (device, metric) or a metric query; nothing here
knows a vendor. Devices hidden or renamed in settings are honoured.

Fonts: the Go fonts (`golang.org/x/image/font/gofont`, WGL4 → Cyrillic
covered) via `opentype`, pure Go, embedded in the binary. Golden-image tests
per widget; `golang.org/x/image` is pinned and bumped deliberately, because a
rasteriser change moves pixels.

### Icons (`internal/icon`)

Pictograms are part of the design, not decoration: on e-ink a glyph reads
faster than a label, and every tile carries one. Icons are **glyphs of an
icon font**, rasterised by the same `opentype` path as text — no new
dependency, no SVG renderer, any size (a tile on 800×480 and on a small mono
panel get icons of their own size), theme colours apply as to text. The set
is **Material Design Icons (Pictogrammers), Apache 2.0**: filled style,
which survives 1-bit thresholding at 24 px where outline sets (Tabler,
Lucide) fall apart; it covers weather (`weather-sunny/cloudy/rainy/snowy/
windy/fog`, `weather-sunset-up/down`, `weather-night`), home devices
(`thermometer`, `water-percent`, `battery-10…100`, `lock`/`lock-open`,
`power-plug`/`power-plug-off`, `water`/`water-off`, `washing-machine`,
`wifi-off`, `cloud-off`), rooms (`bed`, `sofa`, `desk`, `stove`) and signals
(`alert`, `bell`, trend arrows). Fallback with the same properties: Phosphor
Fill (MIT).

The package embeds a **subset** of the font (~11 KB for ~70 glyphs) holding
only the glyphs named in `names.go`, regenerated with `make icons` when an
icon is added: `go run ./internal/icon/cmd/codepoints` lists the code points
and `tools/subset-icons.mjs` (Node, the `subset-font` package) cuts the
upstream TTF — the one non-Go development tool, used like mockgen, never at
runtime; the font's licence ships next to it. (Embedding the full 1.3 MB
TTF is the fallback if the Node step is unwanted.) It exposes
typed names (`icon.Thermometer`, `icon.LockOpen`, `icon.WeatherRainy`),
tables `WMO weather code → icon`, `battery level → icon`, `device Kind →
default icon`, and the whitelist of names a settings override may use
(`icon: bed` on the bedroom sensor). The canvas gets `Icon(name, size,
colour, at)`. A unit test asserts every typed name resolves to a real glyph,
not `.notdef`, so a subset regenerated without a new icon fails the build,
not the frame.

Pages: a display's config lists one or more scenes; the button cycles them,
otherwise page 0 is shown (a `rotate_every` option for auto-cycling, a
day/night pair via settings).

```yaml
displays:
  - kind: epd7in5b
    spi: /dev/spidev0.0
    pins: {dc: GPIO25, rst: GPIO17, busy: GPIO24}
    scenes: [overview, climate]
    tick: 5m
    full_every: 1h
  - kind: httpview
    scenes: [overview]
```

## HTTP server and back office (`internal/httpd`)

One `net/http` server on a LAN address, `go:embed` static files, server-side
templates, no JS framework. Features register routes on it instead of editing
one handler file:

```go
type Registrar interface {
    Handle(pattern string, h http.Handler) // "GET /alerts", Go 1.22 patterns
}
```

Pages, in the order they will appear: frames of every display (`httpview`),
device state, alerts with acknowledge, events timeline, history charts
(server-rendered SVG), settings (intervals, rules, scenes, quiet hours) with
apply-without-restart, force refresh / next page, source health and request
counters, logs tail, CSV export of history, `/metrics` in Prometheus text
format for a scraper on the mini-PC. Auth: a single password (basic auth over
the LAN; the box is not exposed to the internet).

## Advisor (`internal/advisor`)

```go
type Advisor interface {
    Summarise(ctx context.Context, c Context) (string, error)
}

// Context is what the context builder assembles: the snapshot, the events of
// the period, daily extremes and trends from the store, active alerts.
```

The first implementation talks to an OpenAI-compatible endpoint on the
mini-PC (Ollama and llama.cpp both serve one). Uses: a morning digest on the
screen (`Note` on the virtual device `advisor:home`, shown by the `Text`
widget) and in Telegram, a one-line comment attached to an urgent alert,
answers to questions asked in Telegram. Runs on a schedule and on demand;
never in the render path.

## Telegram (`internal/notify/telegram`)

Outbound: alerts by severity, morning digest. Inbound (later): `/status`,
`/frame` (sends the current PNG), `/mute 1h`, `/ack`, free-text questions
routed to the advisor. Inbound commands go through `Commands`.

## Localisation (`internal/i18n`)

English and Russian from the first frame, on `nicksnyder/go-i18n/v2` (the
de-facto standard; the `x/text/message` tooling is heavier and generates
code). Catalogues `locales/en.yaml` and `locales/ru.yaml` are embedded in
the binary, keys namespaced by surface (`scene.humidity`,
`alert.leak.raised`, `web.settings.title`, `telegram.digest.header`), one
`Bundle` per language wrapping a go-i18n `Localizer`: `T(key, Args{...})`
for messages with named placeholders (`"updated {{.Time}}"`, so a
translation may reorder them), `N(key, n)` for plurals (the CLDR form comes
from the library; the Russian catalogue carries `one/few/many`, the English
one `one/other`) and locale-aware number and date formatting (`22,4 °C` vs
`22.4 °C`, `13.09.2026` vs `2026-09-13`). A key missing in a language falls
back to English; a key missing everywhere renders as the key itself, so a
gap is visible rather than fatal. The language is a setting: the coordinator asks a `Locale`
(`render.FixedLocale` now, the settings layer later) before every frame and
carries the bundle in the `View`, so different displays may speak different
languages and Telegram and the back office use the same bundles. Vendor device names stay
as the user named them in the apps. A unit test asserts both catalogues
have the same key set.

## Runtime

```
Pollers / Watchers ──Sink: Batch, Event, Health──▶ core ──changes/events──▶ storage
Button / httpd / Telegram ──Commands──────────────▶  │  ──alert transitions─▶ telegram, actions, render
                                                     ▼ (tick, or an alert transition)
                                        render coordinators (1/display) ──▶ Display.Show
```

The core goroutine owns State; render coordinators (`internal/render`, one
per display) take snapshots, render, apply the refresh policy and call `Show` (which may block 19 s — that is why
each display has its own goroutine). Flags: `-config`, `-once` (poll, render
every display once, exit), later `-preview out.png` (render scenes from a
JSON fixture without any source — the dev loop on the PC). SIGTERM cancels
the root context: modules stop bottom-up, the panel goes to sleep. A systemd
unit with `LoadCredential=` lines (see Secrets).

## Startup, deployment, unattended operation

An appliance nobody watches. The rules:

- **Clock sanity before history.** The Pi has no RTC; until NTP syncs,
  `time.Now()` may be 1970. The unit has `After=time-sync.target`, and the
  core additionally refuses to persist, journal or prune anything while the
  clock reads earlier than the binary's own modification time (the core
  module's `clockFloor`: a build cannot be older than itself) — readings
  are applied to the State but not written, with one warning.
- **Restore, then poll.** State is seeded from the last stored point of
  every series (and, from stage 4, the active alerts), so the first frame
  after a reboot is the last known truth with a "restored…" stamp in the
  header instead of "updated", until the first live batch replaces the
  picture.
- **Supervisor-agnostic liveness.** The unit is `Type=notify`: the
  application reports `READY=1` once every module started, `STOPPING=1` on
  shutdown, and feeds `WATCHDOG=1` at half of `WatchdogSec` through
  `coreos/go-systemd/v22/daemon` (no-ops without `NOTIFY_SOCKET`, so Docker
  and `-once` are unaffected). Stage 3 adds the check itself: a goroutine
  verifies that the core loop and every render coordinator have made
  progress within `2 × tick`, gates the watchdog ping on it, and exits
  non-zero without systemd — whatever supervises the process restarts it,
  systemd (`Restart=always`) or Docker (`restart: unless-stopped`) alike.
- **Two packagings, one binary.** `deploy/systemd/domovoi.service`
  (`Type=notify`, `Restart=always`, `WatchdogSec=120`,
  `WorkingDirectory=/var/lib/domovoi`, `LoadCredential=` lines,
  `After=time-sync.target network-online.target`, the `domovoi` user in
  `spi`/`gpio`) and `deploy/docker/compose.yaml` (a `FROM scratch` image
  with the static binary, ~15 MB; `network_mode: host` for the LAN server;
  `devices:` for `/dev/spidev0.0` and `/dev/gpiochip0` with `group_add`;
  Compose file-based `secrets:` mounted at `/run/secrets`; the json-file log
  driver capped with `max-size`). The application sees no difference:
  secrets are files in a directory, config is a mounted file, liveness is
  the exit code. **Decision: systemd on the Pi Zero 2W** — `dockerd` +
  `containerd` would cost roughly 100 MB of its 512 MB and idle CPU for no
  gain with one static binary. The Compose packaging is kept as an
  optional second target (a mini-PC, another box), built from the same
  image recipe; the design does not care which supervisor runs it.
- **Disk full**: the journal size is capped (`SystemMaxUse=`), readings
  writes that fail are logged once and dropped — the display keeps working
  from the State.
- **Flags**: `-config`, `-check` (load and validate the config, resolve
  secrets, build every instance, exit 0/1 — run it before every restart),
  `-once`, `-version` (set with `-ldflags -X`), later `-preview`. `-check`
  touches nothing outside the process: `container.Check` is set, and a
  module whose singleton would create a file or open hardware validates
  instead — the storage module verifies the database directory exists and
  migrates an in-memory database, so `sudo domovoi -check` leaves no
  root-owned file for the service user to trip over.
- **Release**: `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build`, `scp` the
  binary and the unit file, `domovoi -check`, `systemctl restart` (the Pi
  path); `docker build` (multi-arch, `FROM scratch`) + `docker compose up
  -d` for the optional container target. A
  `Makefile` with `build`, `test`, `test-integration`, `lint`, `image`,
  `release` targets; the user runs the deploy.
- **Backup**: nightly `VACUUM INTO` of the database to the mini-PC.

## Dependencies

Standard library by default; own code where the protocol is simple; a
third-party module only where rewriting would be a project of its own. All
pure Go, no cgo.

| Module | For | Stage | Why |
|---|---|---|---|
| `mshaulsky/tuyacloud` | climate sensors | 1 | ours |
| `go.yaml.in/yaml/v3` | config | 1 | successor of the archived `gopkg.in/yaml.v3`, same API |
| `golang.org/x/image` | `gofont` + `opentype`, `draw` | 1 | the pure-Go TTF rasteriser; Go fonts cover Cyrillic |
| `go.uber.org/mock` | test mocks | 1 | tests only, `tool` directive |
| `github.com/nicksnyder/go-i18n/v2` | message catalogues | 1 | the de-facto standard: CLDR plural rules for every language, YAML message files, named template placeholders |
| `github.com/coreos/go-systemd/v22` (`daemon` only) | systemd readiness and watchdog | 1 | the reference sd_notify client, pure Go; no-op outside systemd |
| `github.com/prometheus/client_golang` | metrics | 1 | the official client: counters, histograms, `promhttp`, Go/process collectors; +2.0 MB measured |
| `mshaulsky/aqaramcp` | lock, leaks, outlet | 2 | ours |
| `modernc.org/sqlite` | history, events, settings | 2 | the proven cgo-free SQLite, arm64 OK; ~9 MB of binary |
| `github.com/pressly/goose/v3` | schema migrations | 2 | standard format, Go migrations, CLI optional; +1.3 MB measured |
| `mshaulsky/waveshare-epd7in5b-v2` + `periph.io/x/{conn,host}/v3` | the panel, SPI/GPIO | 3 | ours; periph arrives transitively |
| `github.com/eclipse/paho.golang` | MQTT for Zigbee2MQTT | 5 | actively developed, `autopaho` reconnects |
| `github.com/skip2/go-qrcode` | QR on screen | later | small, pure Go |

Assets, not modules: the Go fonts (already in `x/image`) and a subset of
the Material Design Icons font (Apache 2.0, licence shipped alongside);
`tools/subset-icons.mjs` (Node + `subset-font`) is a development-time tool
for regenerating the subset, never a runtime dependency.

Deliberately hand-written instead of imported: the Telegram Bot API (three
JSON calls), the OpenAI-compatible LLM request, Open-Meteo, SVG charts, and
the HTTP server (`net/http` 1.22 patterns,
`html/template`, `go:embed`). With `Start`/`Stop` there is no need for
`errgroup`.

## Code conventions

- **`interfaces.go` in every package** declares everything the package
  depends on (the vendor client, `History`, `Journal`, `Metrics`, the panel
  driver) and carries the `go:generate` line for mockgen
  (`go tool mockgen -source=interfaces.go -destination=mocks_test.go`).
  Unit tests use only those mocks. The file doubles as the package's
  dependency list.
- **Metrics behind interfaces.** A package states what it needs to count in
  its own terms (`IncPollSuccess(source)`, `IncEvent(kind)`,
  `IncRender(display, mode)`, `ObserveRenderDuration`), in
  `interfaces.go`. One implementation in `internal/metrics` satisfies all of
  them on top of `prometheus/client_golang` and exposes `Handler()` for
  httpd to mount at `/metrics`; only that package imports the client.
  Durations are histograms with buckets sized for the workload (a poll is a
  couple of round trips, a render includes the 12–19 s panel refresh); the
  Go runtime and process collectors are registered there too, so the Pi's
  memory, GC and file descriptors come for free. Adapters count upstream
  requests through the same door (`IncRequest`). A package that needs a
  private wrapper keeps it in its own `metrics.go`.
- **Current idioms, enforced.** `sync.WaitGroup.Go`, `slices`/`maps`
  (`slices.Sorted(maps.Keys(m))`, `slices.Backward`, `cmp.Or` comparators),
  `for i := range n`, `t.Context()` in tests, `context.WithoutCancel` for
  shutdown work. golangci-lint runs `modernize`, `intrange` and `usetesting`
  so older forms do not creep back in.
- **Time is virtual in tests, not injected.** Code uses `time.Now()`,
  `time.After` and friends directly; tests of anything time-dependent
  (pollers, the render tick, `for:` timers, heartbeat, quiet hours) run
  inside `testing/synctest` bubbles (Go 1.25), where the clock starts at
  2000-01-01 and advances only when every goroutine is blocked, so a
  "wait one minute" test takes no time and is exact. No clock interfaces,
  no fake clocks. Subtests open their own bubble (`t.Run` inside a bubble
  is not allowed). Functions that compute from a moment take it as a
  parameter (`Snapshot(now)`).
- Errors wrapped with context, library code never logs, declaration order
  struct → consts → vars → helper types → constructor → public → private,
  English everywhere in the repository.

## Testing

- **Unit tests**: table-driven (`name`/`want`/`wantErr`), one test per
  public method, dependencies mocked from `interfaces.go`. Adapters test
  against the client libraries' fake servers where those exist.
- **Integration tests**: `*_integration_test.go` behind `//go:build
  integration`, for everything that touches real infrastructure — the
  repositories on `sqlite.Open(":memory:")` with the real migrations,
  `pngfile` writing a file, `httpd` listening on a port. Happy path only;
  database failures are not simulated, so repositories need no mocks. CI
  runs both `go test ./...` and `go test -tags integration ./...` (no
  external services, the tag is for the local edit-run loop).
- policy, alerts, state: pure table tests; scenes: golden PNGs (`-update`
  flag); core: fake Source/Display, the pipeline end-to-end in-process.

## Planned features (not built yet, hooks in place)

| Feature | Needs |
|---|---|
| AI digest and Q&A via the local LLM | `Advisor`, event journal, history queries, `Note` metric |
| Back office (settings, alerts, timeline, charts, logs) | `httpd` registrar, settings layer, `Commands` |
| Two-way Telegram | `Commands`, `httpview` frame, advisor |
| Derived metrics (dew point, ventilate hint, daily energy) | virtual sources with a state reader, counter delta query |
| Calendar (ICS) | virtual source |
| Automation (leak → outlet off) | `Controller`, rule `action` |
| Presence (home/away mode) | virtual source + settings-driven cadence |
| Button gestures, day/night scenes, ghosting wash cycle, QR, timelapse | `Commands`, quiet hours in the policy, frame archive |
| Pi self-monitoring and quota tracking | `IncRequest` counters, `system` source, `/metrics` |
| Scenes defined in YAML | widgets bound by (device, metric), scene as data |
| Compound alert conditions | `all:` list inside a rule |
| Second account of the same vendor | source `name` distinct from `kind` |

## Delivery order

1. container, application (modules, run), model (readings + events, metric
   catalogue), source (Sink, Batch, Poller with health), state
   (health, stale), config (file layer, `Secret`, `${NAME}` resolution,
   `timezone`, `-check`), i18n (en + ru), icon font subset and the `icon`
   package, tuya adapter with request counting, `pngfile` display, render
   coordinator (tick-driven, full refresh only), the `overview` scene per
   the product spec — runs on the PC, draws real Tuya data to a PNG.
2. aqara adapter (verify the unlocked `lock_state`), **weather source
   (Open-Meteo) and the outside tile**, storage (sqlite + goose migrations,
   repositories for readings, devices, events, settings), write policy in
   the core, restore-on-start, event classifier, sparklines and daily
   extremes.
3. `epd7in5b` display + refresh policy (partial, fast, failed-show
   recovery, quiet hours) — first frames on the Pi; systemd unit with
   watchdog, credentials and time-sync ordering; clock-sanity guard;
   `Makefile` release flow.
4. alerts (numeric conditions, quiet hours) + Telegram (outbound) +
   `httpview` + minimal `httpd` (frames, state, health) with the route
   registrar and `Commands`.
5. mqtt source (when the SLZB-06U arrives), button events, pages.
6. back office pages and runtime settings, reload; then advisor, virtual
   sources and the rest of the planned list as appetite dictates.
