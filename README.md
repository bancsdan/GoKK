<div align="center">

# 🚌 bkk

**Next real-time BKK departures, straight from your terminal.**

Type a route and a rough stop name. Get the next buses, trams and metros for
every direction in under a second. No app, no browser, no map.

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Zero deps](https://img.shields.io/badge/dependencies-none-success)](go.mod)
[![Data: BKK FUTÁR](https://img.shields.io/badge/data-BKK%20FUT%C3%81R%20OpenData-1d5fa7)](https://opendata.bkk.hu)

<img src="assets/demo.gif" alt="bkk demo: real-time departures for route 155 at Virányos út, grouped by direction" width="720">

</div>

---

## ✨ Why

- ⚡ **Glanceable.** One line per direction, seconds to departure, nothing else.
- 🔎 **Fuzzy stops.** `viranyos` finds *Virányos út*, `szell kalman` finds *Széll Kálmán tér M*. Accents optional.
- 📡 **Live data.** Real-time predictions from the BKK FUTÁR feed; schedule-only entries are marked with `~`.
- 🚀 **Fast.** Route and stop data is cached for a day, so after the first run only one network call is made.
- 📦 **Zero dependencies.** Standard library only, single binary, `go install`-able.

## 🏁 Quick start

```sh
go install github.com/bancsdan/GoKK/cmd/bkk@latest
export BKK_API_KEY=your-key        # free key from https://opendata.bkk.hu
bkk 155 viranyos -c 3
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

Requires Go 1.22+.

```sh
go install github.com/bancsdan/GoKK/cmd/bkk@latest
```

Or from a checkout:

```sh
go build -o bkk ./cmd/bkk
```

The result is a single binary with no third-party or runtime dependencies.

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
bkk <route> <stop-query> [-c N] [-t] [-r]
bkk <route> -l
```

| Argument / flag   | Meaning                                                                 |
|-------------------|-------------------------------------------------------------------------|
| `<route>`         | Route short name as riders know it: `155`, `4`, `M2`, `9`, `7E`         |
| `<stop-query>`    | Free-text stop name, accent-insensitive and fuzzy                       |
| `-c`, `--count N` | Departures to show per direction (default 1)                            |
| `-t`, `--times`   | Also print clock times: `5m42s (22:41)`                                 |
| `-l`, `--list`    | List the route's stops by direction instead of arrivals                 |
| `-r`, `--refresh` | Ignore the cached route and stop data                                   |
| `-h`, `--help`    | Show help                                                               |

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

Standard library only; no third-party dependencies.

---

<div align="center">
Data © <a href="https://opendata.bkk.hu">BKK FUTÁR OpenData</a>. Not affiliated with BKK.
</div>
