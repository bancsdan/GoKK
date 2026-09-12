package app

import (
	"fmt"
	"strings"
	"time"
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
