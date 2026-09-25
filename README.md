<div align="center">

# 🚌 bkk

**Next real-time BKK departures, straight from your terminal.**

Type a route and a rough stop name. Get the next buses, trams and metros for
every direction in under a second. No app, no browser, no map.

[![CI](https://github.com/bancsdan/GoKK/actions/workflows/ci.yml/badge.svg)](https://github.com/bancsdan/GoKK/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Zero deps](https://img.shields.io/badge/dependencies-none-success)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Data: BKK FUTÁR](https://img.shields.io/badge/data-BKK%20FUT%C3%81R%20OpenData-1d5fa7)](https://opendata.bkk.hu)

<img src="assets/demo.gif" alt="bkk demo: fuzzy stop lookup, departures with clock times, saving a lookup as an alias with --save, replaying it with bkk home, and JSON output piped to jq" width="720">

</div>

---

## ✨ Why

- **Glanceable.** One line per direction, seconds to departure, nothing else.
- **Fuzzy stops.** `viranyos` finds *Virányos út*, `szell kalman` finds *Széll Kálmán tér M*. Accents optional.
- **Live data.** Real-time predictions from the BKK FUTÁR feed; schedule-only entries are marked with `~`.
- **Fast.** Route and stop data is cached for a day, so after the first run only one network call is made.
- **Aliases.** Add `--save home` to any lookup and `bkk home` repeats it; bare `bkk` runs your default.
- **Scriptable.** `--json` for status bars, launchers and scripts.
- **Zero dependencies.** Standard library only, single binary, `go install`-able.

## 🏁 Quick start

```sh
go install github.com/bancsdan/GoKK/cmd/bkk@latest
export BKK_API_KEY=your-key        # free key from https://opendata.bkk.hu
bkk 155 viranyos -c 3 --save home  # look it up, and remember it as "home"
bkk home                           # from now on
```

```
Virányos út
155 → Fácános tér
  3m12s  7m0s   16m30s
155 → Széll Kálmán tér M
  ~5m0s  12m24s
```

Every value is the time until the vehicle leaves (`42s`, `5m42s`, `1h3m11s`).
`~` means no live prediction yet, so the scheduled time is shown. `now` means
it is due.

## 📥 Install

Homebrew (macOS and Linux):

```sh
brew install bancsdan/tap/bkk
```

Prebuilt binaries for macOS, Linux and Windows are attached to every
[GitHub release](https://github.com/bancsdan/GoKK/releases). With Go 1.22+
installed:

```sh
go install github.com/bancsdan/GoKK/cmd/bkk@latest
```

Or from a checkout:

```sh
go build -o bkk ./cmd/bkk
```

The result is a single binary with no third-party or runtime dependencies.
`bkk --version` prints the version, commit and build date of the binary you
have.

## 🔑 API key

`bkk` needs a free FUTÁR OpenData key. Register at
<https://opendata.bkk.hu>, create a key, then provide it either way:

```sh
export BKK_API_KEY=your-key                                  # env var, or
mkdir -p ~/.config/bkk && echo your-key > ~/.config/bkk/api_key   # config file
```

The env var wins when both are set. `$XDG_CONFIG_HOME/bkk/api_key` is used
if `XDG_CONFIG_HOME` is set. The key is only ever sent to `futar.bkk.hu`, as
the `key` query parameter the API requires. Cached lookups (like `-l` on a
route you have already queried) work without a key.

## 🧭 Usage

```
bkk <route> <stop-query> [-c N] [-H Q] [-X Q] [-t] [-j] [-r] [-s NAME]
bkk <route> -l [-j]
bkk <alias> [flags]
```

| Argument / flag     | Meaning                                                                 |
|---------------------|-------------------------------------------------------------------------|
| `<route>`           | Route short name as riders know it: `155`, `4`, `M2`, `9`, `7E`         |
| `<stop-query>`      | Free-text stop name, accent-insensitive and fuzzy                       |
| `<alias>`           | A saved command, see [Aliases](#aliases)                                |
| `-c`, `--count N`   | Departures to show per direction (default 1)                            |
| `-H`, `--heading Q` | Only show departures toward headsign `Q`; fuzzy, repeatable             |
| `-X`, `--not-heading Q` | Hide departures toward headsign `Q`; fuzzy, repeatable              |
| `-t`, `--times`     | Also print clock times: `5m42s (22:41)`                                 |
| `-j`, `--json`      | Print JSON instead of text (see below)                                  |
| `-l`, `--list`      | List the route's stops by direction instead of arrivals                 |
| `-r`, `--refresh`   | Ignore the cached route and stop data                                   |
| `-s`, `--save NAME` | Run the command, then save it as alias `NAME`                           |
| `-a`, `--aliases`   | Show the configured aliases                                             |
| `-h`, `--help`      | Show help                                                               |

Flags may appear anywhere: `bkk 155 viranyos -c 3` works.

### Examples

<details open>
<summary><b>Next departure in each direction</b></summary>

```
$ bkk 155 viranyos
Virányos út
155 → Fácános tér
  3m12s
155 → Széll Kálmán tér M
  ~5m0s
```
</details>

<details>
<summary><b>More departures, with clock times</b></summary>

```
$ bkk 4 moricz -c 2 -t
Móricz Zsigmond körtér M
4 → Széll Kálmán tér M
  2m5s (22:41)   9m40s (22:48)
4 → Újbuda-központ M
  now (22:39)    6m18s (22:45)
```
</details>

<details>
<summary><b>Only some headings</b></summary>

A line can have several termini in the same direction. `-H` keeps the
headsigns you name and `-X` drops them. Both match like stop queries.

```
$ bkk h5 aquincum -X batthyany
Aquincum
H5 → Békásmegyer
  9m55s
H5 → Szentendre
  ~17m55s
```
</details>

<details>
<summary><b>List a route's stops</b></summary>

```
$ bkk 155 -l
155
Széll Kálmán tér M → Fácános tér
   1. Széll Kálmán tér M
   2. Nyúl utca
   3. Városmajor
  ...
  17. Fácános tér
Fácános tér → Széll Kálmán tér M
   1. Fácános tér
   2. Csillagvölgyi út
  ...
```
</details>

<details>
<summary><b>Ambiguous or unknown stop</b></summary>

```
$ bkk 155 v
"v" matches several stops on route 155; be more specific:
  Városmajor
  Virányos út
  Kútvölgyi lejtő, Traumatológia
  Csillagvölgyi út

$ bkk 155 nowhere
no stop matching "nowhere" on route 155. Stops on this route:
  Széll Kálmán tér M
  ...
```
</details>

<details>
<summary><b>Late at night</b></summary>

```
$ bkk 155 viranyos
Virányos út
No upcoming 155 departures in the next 90 min.
```
</details>

Directions are labelled with the trip headsign, the terminus the vehicle is
heading to. A stop served in one direction only shows one group. Output is
coloured on a terminal and plain when piped or when `NO_COLOR` is set.

### Aliases

Once a lookup looks right, add `--save NAME` to remember it:

```
$ bkk 155 viranyos -c 3 -t --save home
Virányos út
155 → Fácános tér
  3m12s (22:39)   7m0s (22:43)    16m30s (22:52)
155 → Széll Kálmán tér M
  ~5m0s (22:41)   12m24s (22:48)
saved alias home = 155 viranyos -c 3 -t (~/.config/bkk/config)

$ bkk home
Virányos út
...
```

The command runs first and is only saved if it succeeds, so a typo in the
stop name never becomes an alias. Saving an existing name replaces it.
`bkk -a` lists what is configured.

Aliases are plain lines in `~/.config/bkk/config` (or
`$XDG_CONFIG_HOME/bkk/config`), `name = args`, so you can also edit the
file by hand. Lines starting with `#` are comments.

```ini
home    = 155 viranyos -c 3 -t
work    = 4 moricz
default = home
```

Plain `bkk` runs the `default` alias. Flags after an alias override the
alias's own, so `bkk home -c 1` shows one departure. Saving from an alias
records the expanded command: `bkk home -j --save home-json` stores
`155 viranyos -c 3 -t -j`.

### JSON output

`-j` prints one JSON document, for scripts, launchers and status bars.
Times are RFC 3339, `inSeconds` counts from `generatedAt` (the server's
clock), and `live` is `false` for schedule-only entries. Colour is never
emitted in JSON mode.

```sh
$ bkk 155 viranyos -c 2 -j
{
  "stop": "Virányos út",
  "stopIds": ["BKK_F02464", "BKK_F02465"],
  "route": "155",
  "generatedAt": "2026-09-13T08:41:07+02:00",
  "directions": [
    {
      "direction": "0",
      "headsign": "Fácános tér",
      "departures": [
        { "at": "2026-09-13T08:44:19+02:00", "inSeconds": 192, "live": true },
        { "at": "2026-09-13T08:48:07+02:00", "inSeconds": 420, "live": true }
      ]
    },
    {
      "direction": "1",
      "headsign": "Széll Kálmán tér M",
      "departures": [
        { "at": "2026-09-13T08:46:07+02:00", "inSeconds": 300, "live": false }
      ]
    }
  ]
}
```

`bkk 155 -l -j` prints the stop list the same way: an array of routes, each
with `directions[].from`, `to` and `stops[]` carrying stop `id` and `name`.
No upcoming departures gives `"directions": []`; errors still go to stderr
as text with exit status 1. A one-liner for a status bar:

```sh
bkk home -j | jq -r '.directions[] | "\(.headsign[:12]) \(.departures[0].inSeconds / 60 | floor)m"'
```

## ⚙️ How it works

<details>
<summary>Route resolution, fuzzy matching, arrivals and caching</summary>

1. **Route resolution.** The `search` endpoint is queried with the route
   name and the results are filtered to routes whose `shortName` matches
   exactly (case- and accent-insensitive). If several routes share a name
   (a bus and a tram, say) all of them are used.
2. **Stops.** `route-details` lists each route variant with its ordered stop
   IDs and headsign; stop names come from the response references.
3. **Fuzzy match.** The query and every stop name are lowercased and
   stripped of accents. Exact name wins, then substring, then a small
   Levenshtein distance against the name or a run of its words. All platform
   IDs sharing the winning name are collected so both directions are shown.
   If the query matches several distinct stop names, the candidates are
   printed instead of guessing.
4. **Arrivals.** One `arrivals-and-departures-for-stop` call with all
   matched stop IDs and a 90-minute window. Entries are filtered to the
   route, non-boardable and already-departed trips are dropped, and each
   trip's `predictedDepartureTime` is used when present (falling back to
   `predictedArrivalTime`, then the scheduled time with a `~`).

Endpoint paths and parameters follow the published OpenAPI spec:
<https://opendata.bkk.hu/docs/futar-openapi.yaml>.

**Caching.** Route lookups and route→stop lists are cached as JSON for
24 hours in `~/.cache/bkk` (`$XDG_CACHE_HOME/bkk` if set). After the first
run only the live arrivals call hits the network. Live arrivals are never
cached. Delete the directory or pass `-r` to refresh.

**Errors.** Every failure is a one-line message on stderr with exit
status 1: missing or rejected API key, network timeout (10 s budget per
run), unknown route (with similar route names), no or ambiguous stop match
(with the route's stops). "No upcoming departures" is printed normally with
exit status 0. Set `BKK_DEBUG=1` to print every request URL and HTTP status
to stderr.
</details>

## 🛠 Development

```sh
go test ./...                                             # offline, against a fake FUTÁR server
BKK_API_KEY=... go test ./internal/app -run TestLive -v   # one real round-trip
vhs demo.tape                                             # re-render assets/demo.gif
```

The tape points `XDG_CONFIG_HOME` at a temporary directory, so recording
it never touches your own aliases; it needs `bkk` on `PATH` and
`BKK_API_KEY` in the environment.

Standard library only; no third-party dependencies.

---

<div align="center">
<a href="LICENSE">MIT licensed</a>. Data © <a href="https://opendata.bkk.hu">BKK FUTÁR OpenData</a>. Not affiliated with BKK.
</div>
