// Package app wires route resolution, stop matching, the live arrivals call
// and output formatting together.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/bancsdan/GoKK/internal/cache"
	"github.com/bancsdan/GoKK/internal/futar"
	"github.com/bancsdan/GoKK/internal/match"
)

// Options are the per-invocation settings.
type Options struct {
	Route     string   // public short name, e.g. "155"
	Query     string   // fuzzy stop query
	Count     int      // arrivals per direction
	ShowClock bool     // append HH:MM after each minutes value
	Refresh   bool     // bypass the reference-data cache
	List      bool     // list the route's stops instead of arrivals
	JSON      bool     // machine-readable output instead of text
	Headings  []string // fuzzy headsign queries; keep only these directions
	Excludes  []string // fuzzy headsign queries; drop these directions
	Window    time.Duration
}

// App holds the long-lived dependencies.
type App struct {
	Client *futar.Client
	Cache  *cache.Cache
	Out    io.Writer
	Color  bool
}

// UsageError marks errors caused by the user's input (unknown route, no
// stop match, ambiguous query); main prints them without a stack of context.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// Run executes one lookup and writes the result to a.Out.
func (a *App) Run(ctx context.Context, o Options) error {
	if o.Count < 1 {
		o.Count = 1
	}
	if o.Window <= 0 {
		o.Window = 90 * time.Minute
	}
	routes, err := a.resolveRoutes(ctx, o.Route, o.Refresh)
	if err != nil {
		return err
	}
	if o.List {
		lists, err := a.stopLists(ctx, routes, o.Refresh)
		if err != nil {
			return err
		}
		if o.JSON {
			return a.writeJSON(lists)
		}
		a.renderLists(lists)
		return nil
	}
	rs, err := a.routeStops(ctx, routes, o.Refresh)
	if err != nil {
		return err
	}
	cand, err := pickStop(o.Route, o.Query, rs.stops)
	if err != nil {
		return err
	}
	routeIDs := make([]string, len(routes))
	for i, r := range routes {
		routeIDs[i] = r.ID
	}
	arr, err := a.Client.Arrivals(ctx, cand.IDs, routeIDs, o.Window)
	if err != nil {
		return err
	}
	groups := groupArrivals(arr, cand.IDs, routeIDs, rs.dirHeadsigns, o.Count)
	label := routes[0].ShortName
	if label == "" {
		label = o.Route
	}
	if len(o.Headings) > 0 || len(o.Excludes) > 0 {
		groups, err = filterHeadings(groups, headingsAt(cand, rs.stopHeadsigns, groups), label, cand.Name, o)
		if err != nil {
			return err
		}
	}
	if o.JSON {
		return a.writeJSON(arrivalsJSON(cand, label, arr.CurrentTime, groups))
	}
	a.render(cand.Name, label, groups, o)
	return nil
}

func (a *App) writeJSON(v any) error {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// resolveRoutes maps a public short name to every route ID carrying it
// (e.g. a bus and a tram can share a number). Cached by short name.
func (a *App) resolveRoutes(ctx context.Context, short string, refresh bool) ([]futar.Route, error) {
	key := "route-" + match.Normalize(short)
	var routes []futar.Route
	if !refresh && a.Cache.Load(key, &routes) && len(routes) > 0 {
		return routes, nil
	}
	found, err := a.Client.SearchRoutes(ctx, short)
	if err != nil {
		return nil, err
	}
	want := match.Normalize(short)
	var names []string
	for _, r := range found {
		if match.Normalize(r.ShortName) == want {
			routes = append(routes, r)
		} else if r.ShortName != "" && len(names) < 8 {
			names = append(names, r.ShortName)
		}
	}
	if len(routes) == 0 {
		msg := fmt.Sprintf("unknown route %q", short)
		if len(names) > 0 {
			msg += " (did you mean: " + strings.Join(uniq(names), ", ") + ")"
		}
		return nil, &UsageError{msg}
	}
	_ = a.Cache.Store(key, routes)
	return routes, nil
}

// routeData is what routeStops gathers from the route details.
type routeData struct {
	stops         []match.Stop        // every platform, travel order, direction 0 first
	dirHeadsigns  map[string]string   // directionId → headsign fallback
	stopHeadsigns map[string][]string // stop ID → headsigns of variants departing from it
}

// routeStops collects the platforms and headsigns served by the routes.
// Cached per route ID.
func (a *App) routeStops(ctx context.Context, routes []futar.Route, refresh bool) (*routeData, error) {
	var stops []match.Stop
	seen := map[string]bool{}
	headsigns := map[string]string{}
	stopHeadsigns := map[string][]string{}
	for _, r := range routes {
		rd, err := a.details(ctx, r, refresh)
		if err != nil {
			return nil, err
		}
		for _, v := range rd.Variants {
			if _, ok := headsigns[v.Direction]; !ok && v.Headsign != "" {
				headsigns[v.Direction] = v.Headsign
			}
			for i, id := range v.StopIDs {
				// A variant's last stop is where it terminates, not a heading
				// anyone can board toward.
				if v.Headsign != "" && i < len(v.StopIDs)-1 && !containsStr(stopHeadsigns[id], v.Headsign) {
					stopHeadsigns[id] = append(stopHeadsigns[id], v.Headsign)
				}
				if seen[id] {
					continue
				}
				seen[id] = true
				name := rd.Stops[id].Name
				if name == "" {
					name = id
				}
				stops = append(stops, match.Stop{ID: id, Name: name})
			}
		}
	}
	if len(stops) == 0 {
		return nil, &UsageError{fmt.Sprintf("route %s has no stops in the timetable", routes[0].ShortName)}
	}
	return &routeData{stops: stops, dirHeadsigns: headsigns, stopHeadsigns: stopHeadsigns}, nil
}

// details returns a route's variants (sorted direction 0 first) and stops,
// from the cache when fresh.
func (a *App) details(ctx context.Context, r futar.Route, refresh bool) (*futar.RouteDetails, error) {
	key := "stops-" + r.ID
	var rd futar.RouteDetails
	if refresh || !a.Cache.Load(key, &rd) || len(rd.Variants) == 0 {
		got, err := a.Client.RouteDetails(ctx, r.ID)
		if errors.Is(err, futar.ErrNotFound) {
			return nil, &UsageError{fmt.Sprintf("route %s (%s) has no details; try -r to refresh the cache", r.ShortName, r.ID)}
		}
		if err != nil {
			return nil, err
		}
		rd = *got
		_ = a.Cache.Store(key, rd)
	}
	sort.SliceStable(rd.Variants, func(i, j int) bool { return rd.Variants[i].Direction < rd.Variants[j].Direction })
	return &rd, nil
}

// RouteStops is one route's stops per direction, for -l.
type RouteStops struct {
	Route      string           `json:"route"`
	RouteID    string           `json:"routeId"`
	Directions []DirectionStops `json:"directions"`
}

// DirectionStops is one direction's stops in travel order. The longest
// variant of a direction is the backbone; stops only served by shorter or
// branch variants are appended after it.
type DirectionStops struct {
	Direction string     `json:"direction"`
	From      string     `json:"from"`
	To        string     `json:"to"`
	Stops     []StopInfo `json:"stops"`
}

// StopInfo is a stop platform.
type StopInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// stopLists builds the -l data for every route sharing the short name.
func (a *App) stopLists(ctx context.Context, routes []futar.Route, refresh bool) ([]RouteStops, error) {
	var out []RouteStops
	for _, r := range routes {
		rd, err := a.details(ctx, r, refresh)
		if err != nil {
			return nil, err
		}
		type dir struct {
			headsign string
			stopIDs  []string
		}
		byDir := map[string]*dir{}
		var order []string
		for _, v := range rd.Variants {
			d, ok := byDir[v.Direction]
			if !ok {
				d = &dir{headsign: v.Headsign}
				byDir[v.Direction] = d
				order = append(order, v.Direction)
			}
			if len(v.StopIDs) > len(d.stopIDs) {
				d.stopIDs = append([]string(nil), v.StopIDs...)
				if v.Headsign != "" {
					d.headsign = v.Headsign
				}
			}
		}
		for _, v := range rd.Variants {
			d := byDir[v.Direction]
			for _, id := range v.StopIDs {
				if !containsStr(d.stopIDs, id) {
					d.stopIDs = append(d.stopIDs, id)
				}
			}
		}
		name := func(id string) string {
			if n := rd.Stops[id].Name; n != "" {
				return n
			}
			return id
		}
		rs := RouteStops{Route: r.ShortName, RouteID: r.ID}
		if rs.Route == "" {
			rs.Route = r.ID
		}
		for _, k := range order {
			d := byDir[k]
			if len(d.stopIDs) == 0 {
				continue
			}
			ds := DirectionStops{Direction: k, From: name(d.stopIDs[0]), To: d.headsign}
			if ds.To == "" {
				ds.To = name(d.stopIDs[len(d.stopIDs)-1])
			}
			for _, id := range d.stopIDs {
				ds.Stops = append(ds.Stops, StopInfo{ID: id, Name: name(id)})
			}
			rs.Directions = append(rs.Directions, ds)
		}
		out = append(out, rs)
	}
	return out, nil
}

// renderLists prints the route name, then for each direction an
// "origin → destination" line and its numbered stops.
func (a *App) renderLists(lists []RouteStops) {
	for _, rs := range lists {
		fmt.Fprintln(a.Out, a.paint(ansiBold, rs.Route))
		for _, d := range rs.Directions {
			fmt.Fprintln(a.Out, a.paint(ansiCyan+ansiBold, d.From+" → "+d.To))
			width := len(fmt.Sprint(len(d.Stops)))
			for i, st := range d.Stops {
				fmt.Fprintf(a.Out, "  %*d. %s\n", width, i+1, st.Name)
			}
		}
	}
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// pickStop fuzzy-matches query against the stops and insists on exactly
// one distinct stop name.
func pickStop(route, query string, stops []match.Stop) (match.Candidate, error) {
	cands := match.Find(query, stops)
	switch len(cands) {
	case 1:
		return cands[0], nil
	case 0:
		all := match.Group(stops)
		var sb strings.Builder
		fmt.Fprintf(&sb, "no stop matching %q on route %s.", query, route)
		if len(all) <= 40 {
			sb.WriteString(" Stops on this route:\n")
			for _, c := range all {
				sb.WriteString("  " + c.Name + "\n")
			}
		} else {
			sb.WriteString(" Closest names:\n")
			for _, c := range match.Closest(query, stops, 8) {
				sb.WriteString("  " + c.Name + "\n")
			}
		}
		return match.Candidate{}, &UsageError{strings.TrimRight(sb.String(), "\n")}
	default:
		var sb strings.Builder
		fmt.Fprintf(&sb, "%q matches several stops on route %s; be more specific:\n", query, route)
		for _, c := range cands {
			sb.WriteString("  " + c.Name + "\n")
		}
		return match.Candidate{}, &UsageError{strings.TrimRight(sb.String(), "\n")}
	}
}

// headingsAt lists the headsigns departing from the candidate's platforms:
// those of the timetable variants, then any extra ones seen in live data.
func headingsAt(cand match.Candidate, stopHeadsigns map[string][]string, groups []Group) []string {
	var out []string
	for _, id := range cand.IDs {
		out = append(out, stopHeadsigns[id]...)
	}
	for _, g := range groups {
		out = append(out, g.Headsign)
	}
	return out
}

// filterHeadings keeps the groups whose headsign matches one of o.Headings
// (all groups when there are none) and drops those matching o.Excludes.
// Each query is matched like a stop query against the headings, and must
// pick exactly one of them.
func filterHeadings(groups []Group, headings []string, route, stopName string, o Options) ([]Group, error) {
	var hs []match.Stop
	for _, h := range headings {
		hs = append(hs, match.Stop{ID: match.Normalize(h), Name: h})
	}
	resolve := func(flag string, queries []string) (map[string]bool, error) {
		set := map[string]bool{}
		for _, q := range queries {
			c, err := pickHeading(flag, q, route, stopName, hs)
			if err != nil {
				return nil, err
			}
			set[match.Normalize(c.Name)] = true
		}
		return set, nil
	}
	keep, err := resolve("--heading", o.Headings)
	if err != nil {
		return nil, err
	}
	drop, err := resolve("--not-heading", o.Excludes)
	if err != nil {
		return nil, err
	}
	var out []Group
	for _, g := range groups {
		h := match.Normalize(g.Headsign)
		if (len(keep) == 0 || keep[h]) && !drop[h] {
			out = append(out, g)
		}
	}
	return out, nil
}

// pickHeading fuzzy-matches query against the headings the way pickStop
// matches stops, insisting on exactly one distinct headsign.
func pickHeading(flag, query, route, stopName string, headings []match.Stop) (match.Candidate, error) {
	cands := match.Find(query, headings)
	if len(cands) == 1 {
		return cands[0], nil
	}
	var sb strings.Builder
	list := cands
	if len(cands) == 0 {
		fmt.Fprintf(&sb, "%s: no heading matching %q for route %s at %s. Headings from this stop:\n", flag, query, route, stopName)
		list = match.Group(headings)
	} else {
		fmt.Fprintf(&sb, "%s: %q matches several headings for route %s at %s; be more specific:\n", flag, query, route, stopName)
	}
	for _, c := range list {
		sb.WriteString("  " + c.Name + "\n")
	}
	return match.Candidate{}, &UsageError{strings.TrimRight(sb.String(), "\n")}
}

// Arrival is one upcoming departure.
type Arrival struct {
	At   time.Time
	Live bool
}

// Group is one direction's upcoming departures.
type Group struct {
	Direction string // directionId, used for ordering
	Headsign  string
	Arrivals  []Arrival
	Now       time.Time
}

// groupArrivals filters stop times to the requested platforms and routes,
// drops departed and non-boardable entries, groups by headsign and keeps
// `count` per group.
func groupArrivals(arr *futar.Arrivals, stopIDs, routeIDs []string, dirHeadsigns map[string]string, count int) []Group {
	wanted := map[string]bool{}
	for _, id := range routeIDs {
		wanted[id] = true
	}
	atStop := map[string]bool{}
	for _, id := range stopIDs {
		atStop[id] = true
	}
	now := arr.CurrentTime
	byKey := map[string]*Group{}
	var order []string
	for _, st := range arr.StopTimes {
		if st.StopID != "" && !atStop[st.StopID] {
			continue
		}
		trip, hasTrip := arr.Trips[st.TripID]
		if hasTrip && !wanted[trip.RouteID] {
			continue
		}
		if !hasTrip && len(wanted) > 0 && len(arr.Trips) > 0 {
			continue // references present but this trip is on another route
		}
		if !st.Boardable() {
			continue
		}
		at, live := st.Predicted()
		if !live {
			at = st.Scheduled()
		}
		if at.IsZero() || at.Before(now.Add(-time.Minute)) {
			continue
		}
		headsign := firstNonEmpty(trip.Headsign, st.StopHeadsign, dirHeadsigns[trip.DirectionID])
		key := trip.DirectionID + "|" + match.Normalize(headsign)
		g, ok := byKey[key]
		if !ok {
			g = &Group{Direction: trip.DirectionID, Headsign: headsign, Now: now}
			byKey[key] = g
			order = append(order, key)
		}
		g.Arrivals = append(g.Arrivals, Arrival{At: at, Live: live})
	}
	groups := make([]Group, 0, len(order))
	for _, k := range order {
		g := byKey[k]
		sort.SliceStable(g.Arrivals, func(i, j int) bool { return g.Arrivals[i].At.Before(g.Arrivals[j].At) })
		if len(g.Arrivals) > count {
			g.Arrivals = g.Arrivals[:count]
		}
		groups = append(groups, *g)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Direction != groups[j].Direction {
			return groups[i].Direction < groups[j].Direction
		}
		return groups[i].Headsign < groups[j].Headsign
	})
	return groups
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func uniq(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
