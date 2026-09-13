package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/bancsdan/GoKK/internal/match"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiCyan   = "\x1b[36m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

func (a *App) paint(code, s string) string {
	if !a.Color || s == "" {
		return s
	}
	return code + s + ansiReset
}

// render prints the grouped arrivals in the compact glance format:
//
//	Zugligeti út
//	155 → Zugliget
//	  3m12s   7m      16m30s
//	155 → Széll Kálmán tér M
//	  ~5m     12m24s
func (a *App) render(stopName, route string, groups []Group, o Options) {
	w := a.Out
	fmt.Fprintln(w, a.paint(ansiCyan, stopName))
	if len(groups) == 0 {
		fmt.Fprintf(w, "No upcoming %s departures in the next %d min.\n", route, int(o.Window.Minutes()))
		return
	}
	for _, g := range groups {
		fmt.Fprintln(w, a.paint(ansiBold, route+" → "+g.Headsign))
		cells := make([]string, 0, len(g.Arrivals))
		width := 6 // "5m42s " — keeps short cells evenly spaced
		for _, ar := range g.Arrivals {
			c := formatArrival(ar, g.Now, o.ShowClock)
			if len([]rune(c)) > width {
				width = len([]rune(c))
			}
			cells = append(cells, c)
		}
		var sb strings.Builder
		sb.WriteString(" ")
		for i, c := range cells {
			pad := strings.Repeat(" ", width-len([]rune(c)))
			color := ansiGreen
			if !g.Arrivals[i].Live {
				color = ansiYellow
			}
			sb.WriteString(" " + a.paint(color, c) + pad)
		}
		fmt.Fprintln(w, strings.TrimRight(sb.String(), " "))
	}
}

// ArrivalsJSON is the machine-readable arrivals output.
type ArrivalsJSON struct {
	Stop        string          `json:"stop"`
	StopIDs     []string        `json:"stopIds"`
	Route       string          `json:"route"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Directions  []DirectionJSON `json:"directions"`
}

// DirectionJSON is one direction's departures.
type DirectionJSON struct {
	Direction  string          `json:"direction"`
	Headsign   string          `json:"headsign"`
	Departures []DepartureJSON `json:"departures"`
}

// DepartureJSON is one departure. InSeconds counts from GeneratedAt and is
// clamped at zero; Live is false for schedule-only entries.
type DepartureJSON struct {
	At        time.Time `json:"at"`
	InSeconds int       `json:"inSeconds"`
	Live      bool      `json:"live"`
}

func arrivalsJSON(cand match.Candidate, route string, now time.Time, groups []Group) ArrivalsJSON {
	out := ArrivalsJSON{Stop: cand.Name, StopIDs: cand.IDs, Route: route, GeneratedAt: now, Directions: []DirectionJSON{}}
	for _, g := range groups {
		d := DirectionJSON{Direction: g.Direction, Headsign: g.Headsign, Departures: []DepartureJSON{}}
		for _, ar := range g.Arrivals {
			in := int(ar.At.Sub(now).Seconds())
			if in < 0 {
				in = 0
			}
			d.Departures = append(d.Departures, DepartureJSON{At: ar.At, InSeconds: in, Live: ar.Live})
		}
		out.Directions = append(out.Directions, d)
	}
	return out
}

// formatArrival renders time-to-arrival like "42s", "5m42s", "1h3m11s" or
// "now", prefixed with ~ when schedule-only and optionally followed by the
// clock time in parentheses.
func formatArrival(ar Arrival, now time.Time, clock bool) string {
	d := ar.At.Sub(now).Truncate(time.Second)
	s := "now"
	if d > 0 {
		s = d.String()
	}
	if !ar.Live {
		s = "~" + s
	}
	if clock {
		s += " (" + ar.At.Local().Format("15:04") + ")"
	}
	return s
}
