package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bancsdan/GoKK/internal/cache"
	"github.com/bancsdan/GoKK/internal/futar"
)

// fakeFutar serves spec-shaped responses for the three endpoints bkk uses
// and records how many times each was hit.
type fakeFutar struct {
	now                       time.Time
	search, details, arrivals atomic.Int32
	lastArrivalsQuery         string
	scheduledOnly             bool
}

func (f *fakeFutar) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "k3y" {
			http.Error(w, "Invalid API key. Please register at https://opendata.bkk.hu", http.StatusUnauthorized)
			return
		}
		env := map[string]any{"code": 200, "currentTime": f.now.UnixMilli(), "status": "OK", "text": "OK", "version": 3}
		switch {
		case strings.HasSuffix(r.URL.Path, "/search"):
			f.search.Add(1)
			q := r.URL.Query().Get("query")
			env["data"] = map[string]any{
				"entry": map[string]any{"routeIds": []string{"BKK_1550", "BKK_1551"}, "query": q},
				"references": map[string]any{"routes": map[string]any{
					"BKK_1550": map[string]any{"id": "BKK_1550", "shortName": "155", "type": "BUS", "description": "Zugliget | Széll Kálmán tér M"},
					"BKK_1551": map[string]any{"id": "BKK_1551", "shortName": "155A", "type": "BUS"},
				}},
			}
		case strings.HasSuffix(r.URL.Path, "/route-details"):
			f.details.Add(1)
			if r.URL.Query().Get("routeId") != "BKK_1550" {
				w.WriteHeader(404)
				return
			}
			env["data"] = map[string]any{
				"entry": map[string]any{
					"id": "BKK_1550", "shortName": "155", "type": "BUS",
					"variants": []map[string]any{
						{"direction": "1", "headsign": "Széll Kálmán tér M", "stopIds": []string{"BKK_F02008", "BKK_F02003", "BKK_F02002"}},
						{"direction": "0", "headsign": "Zugliget", "stopIds": []string{"BKK_F02001", "BKK_F02004", "BKK_F02007"}},
					},
				},
				"references": map[string]any{"stops": map[string]any{
					"BKK_F02001": map[string]any{"id": "BKK_F02001", "name": "Széll Kálmán tér M"},
					"BKK_F02002": map[string]any{"id": "BKK_F02002", "name": "Széll Kálmán tér M"},
					"BKK_F02003": map[string]any{"id": "BKK_F02003", "name": "Zugligeti út"},
					"BKK_F02004": map[string]any{"id": "BKK_F02004", "name": "Zugligeti út"},
					"BKK_F02007": map[string]any{"id": "BKK_F02007", "name": "Zugliget, Libegő"},
					"BKK_F02008": map[string]any{"id": "BKK_F02008", "name": "Zugliget, Libegő"},
				}},
			}
		case strings.HasSuffix(r.URL.Path, "/arrivals-and-departures-for-stop"):
			f.arrivals.Add(1)
			f.lastArrivalsQuery = r.URL.RawQuery
			sec := func(m float64) int64 { return f.now.Add(time.Duration(m * float64(time.Minute))).Unix() }
			st := func(stop, trip string, sched, pred float64) map[string]any {
				m := map[string]any{"stopId": stop, "tripId": trip, "stopHeadsign": "", "departureTime": sec(sched), "serviceDate": "20260912"}
				if pred != 0 && !f.scheduledOnly {
					m["predictedDepartureTime"] = sec(pred)
				}
				return m
			}
			noPickup := st("BKK_F02003", "T_end", 2, 2)
			noPickup["pickupAllowed"] = false
			env["data"] = map[string]any{
				"entry": map[string]any{"stopId": "BKK_F02003", "stopTimes": []map[string]any{
					st("BKK_F02004", "T1", 4, 3.2),
					st("BKK_F02004", "T2", 8, 7),
					st("BKK_F02004", "T3", 15, 16),
					st("BKK_F02004", "T4", 25, 0),
					st("BKK_F02003", "T5", 5, 0),
					st("BKK_F02003", "T6", 12, 12.4),
					st("BKK_F02003", "Tother", 1, 1), // route 155A, must be filtered out
					st("BKK_F02003", "Tgone", -3, -2.5),
					noPickup,
				}},
				"references": map[string]any{"trips": map[string]any{
					"T1":     map[string]any{"id": "T1", "routeId": "BKK_1550", "directionId": "0", "tripHeadsign": "Zugliget"},
					"T2":     map[string]any{"id": "T2", "routeId": "BKK_1550", "directionId": "0", "tripHeadsign": "Zugliget"},
					"T3":     map[string]any{"id": "T3", "routeId": "BKK_1550", "directionId": "0", "tripHeadsign": "Zugliget"},
					"T4":     map[string]any{"id": "T4", "routeId": "BKK_1550", "directionId": "0", "tripHeadsign": "Zugliget"},
					"T5":     map[string]any{"id": "T5", "routeId": "BKK_1550", "directionId": "1", "tripHeadsign": "Széll Kálmán tér M"},
					"T6":     map[string]any{"id": "T6", "routeId": "BKK_1550", "directionId": "1", "tripHeadsign": "Széll Kálmán tér M"},
					"Tother": map[string]any{"id": "Tother", "routeId": "BKK_1551", "directionId": "1", "tripHeadsign": "Elsewhere"},
					"Tgone":  map[string]any{"id": "Tgone", "routeId": "BKK_1550", "directionId": "1", "tripHeadsign": "Széll Kálmán tér M"},
					"T_end":  map[string]any{"id": "T_end", "routeId": "BKK_1550", "directionId": "1", "tripHeadsign": "Széll Kálmán tér M"},
				}},
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(env)
	})
}

func newApp(t *testing.T, f *fakeFutar, key string) (*App, *bytes.Buffer) {
	t.Helper()
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	c := futar.New(key)
	c.BaseURL = srv.URL + "/otp/api/where"
	out := &bytes.Buffer{}
	return &App{Client: c, Cache: &cache.Cache{Dir: t.TempDir(), TTL: time.Hour}, Out: out}, out
}

func TestRunGroupsAndCaches(t *testing.T) {
	f := &fakeFutar{now: time.Date(2026, 9, 12, 22, 30, 0, 0, time.UTC)}
	a, out := newApp(t, f, "k3y")
	ctx := context.Background()

	if err := a.Run(ctx, Options{Route: "155", Query: "zugligeti", Count: 3}); err != nil {
		t.Fatal(err)
	}
	want := "Zugligeti út\n155 → Zugliget\n  3m12s  7m0s   16m0s\n155 → Széll Kálmán tér M\n  ~5m0s  12m24s\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	q := f.lastArrivalsQuery
	for _, needle := range []string{"stopId=BKK_F02004&stopId=BKK_F02003", "includeRouteId=BKK_1550", "minutesAfter=90", "key=k3y", "onlyDepartures=false"} {
		if !strings.Contains(q, needle) {
			t.Errorf("arrivals query %q lacks %q", q, needle)
		}
	}

	// Second run: only the live call may hit the network.
	out.Reset()
	if err := a.Run(ctx, Options{Route: "155", Query: "Zugligeti út", Count: 1}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "Zugligeti út\n155 → Zugliget\n  3m12s\n155 → Széll Kálmán tér M\n  ~5m0s\n" {
		t.Fatalf("count=1 output:\n%s", got)
	}
	if f.search.Load() != 1 || f.details.Load() != 1 || f.arrivals.Load() != 2 {
		t.Fatalf("calls: search=%d details=%d arrivals=%d; want 1/1/2", f.search.Load(), f.details.Load(), f.arrivals.Load())
	}

	// -r bypasses the cache.
	if err := a.Run(ctx, Options{Route: "155", Query: "zugligeti", Refresh: true}); err != nil {
		t.Fatal(err)
	}
	if f.search.Load() != 2 || f.details.Load() != 2 {
		t.Fatalf("refresh should refetch: search=%d details=%d", f.search.Load(), f.details.Load())
	}
}

func TestRunList(t *testing.T) {
	f := &fakeFutar{now: time.Now()}
	a, out := newApp(t, f, "k3y")
	if err := a.Run(context.Background(), Options{Route: "155", List: true}); err != nil {
		t.Fatal(err)
	}
	want := "155\nSzéll Kálmán tér M → Zugliget\n  1. Széll Kálmán tér M\n  2. Zugligeti út\n  3. Zugliget, Libegő\n" +
		"Zugliget, Libegő → Széll Kálmán tér M\n  1. Zugliget, Libegő\n  2. Zugligeti út\n  3. Széll Kálmán tér M\n"
	if out.String() != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	if f.arrivals.Load() != 0 {
		t.Fatal("-l must not call arrivals")
	}
}

func TestRunClockTimesAndColor(t *testing.T) {
	f := &fakeFutar{now: time.Date(2026, 9, 12, 22, 30, 0, 0, time.UTC)}
	a, out := newApp(t, f, "k3y")
	a.Color = true
	if err := a.Run(context.Background(), Options{Route: "155", Query: "zugligeti", ShowClock: true}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	clock := f.now.Add(3*time.Minute + 12*time.Second).Local().Format("15:04")
	if !strings.Contains(got, "3m12s ("+clock+")") || !strings.Contains(got, ansiGreen) || !strings.Contains(got, ansiYellow) {
		t.Fatalf("output:\n%q", got)
	}
}

func TestRunUserErrors(t *testing.T) {
	f := &fakeFutar{now: time.Now()}
	a, _ := newApp(t, f, "k3y")
	ctx := context.Background()
	cases := []struct {
		opts Options
		want string
	}{
		{Options{Route: "999", Query: "x"}, `unknown route "999" (did you mean: 155, 155A)`},
		{Options{Route: "155", Query: "zugliget"}, `"zugliget" matches several stops on route 155; be more specific:` + "\n  Zugligeti út\n  Zugliget, Libegő"},
		{Options{Route: "155", Query: "qqqqqq"}, `no stop matching "qqqqqq" on route 155. Stops on this route:` + "\n  Széll Kálmán tér M\n  Zugligeti út\n  Zugliget, Libegő"},
	}
	for _, c := range cases {
		err := a.Run(ctx, c.opts)
		if err == nil || err.Error() != c.want {
			t.Errorf("%+v:\n got %q\nwant %q", c.opts, err, c.want)
		}
		if _, ok := err.(*UsageError); !ok {
			t.Errorf("%+v: want UsageError, got %T", c.opts, err)
		}
	}
}

func TestRunNoUpcomingAndBadKey(t *testing.T) {
	f := &fakeFutar{now: time.Now()}
	a, out := newApp(t, f, "k3y")
	ctx := context.Background()
	// A stop with only departed/non-boardable trips: Libegő has none at all.
	if err := a.Run(ctx, Options{Route: "155", Query: "libego"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No upcoming 155 departures in the next 90 min.") {
		t.Fatalf("output:\n%s", out.String())
	}

	bad, _ := newApp(t, f, "wrong")
	err := bad.Run(ctx, Options{Route: "155", Query: "zugligeti"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("bad key error = %v", err)
	}
}

func TestFormatArrival(t *testing.T) {
	now := time.Date(2026, 9, 12, 22, 30, 0, 0, time.UTC)
	cases := []struct {
		d    time.Duration
		live bool
		want string
	}{
		{0, true, "now"}, {-20 * time.Second, true, "now"}, {42 * time.Second, true, "42s"},
		{5*time.Minute + 42*time.Second, true, "5m42s"}, {7 * time.Minute, false, "~7m0s"},
		{time.Hour + 3*time.Minute + 11*time.Second + 900*time.Millisecond, true, "1h3m11s"},
	}
	for _, c := range cases {
		if got := formatArrival(Arrival{At: now.Add(c.d), Live: c.live}, now, false); got != c.want {
			t.Errorf("%v live=%v: got %q want %q", c.d, c.live, got, c.want)
		}
	}
}

// TestLive exercises the real API. It runs only when BKK_API_KEY is set:
//
//	BKK_API_KEY=... go test ./internal/app -run TestLive -v
func TestLive(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("BKK_API_KEY"))
	if key == "" {
		t.Skip("BKK_API_KEY not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := futar.New(key)

	// Step 1 of the build plan: one raw arrivals call against a known stop.
	arr, err := c.Arrivals(ctx, []string{"BKK_F01029"}, nil, 90*time.Minute)
	if err != nil {
		t.Fatalf("arrivals: %v", err)
	}
	t.Logf("raw arrivals: %d stop times, %d trips, server time %s", len(arr.StopTimes), len(arr.Trips), arr.CurrentTime.Local().Format(time.RFC3339))
	for i, st := range arr.StopTimes {
		if i >= 3 {
			break
		}
		p, live := st.Predicted()
		t.Logf("  %s trip=%s headsign=%q sched=%s pred=%s live=%v pickup=%v", st.StopID, st.TripID, arr.Trips[st.TripID].Headsign,
			st.Scheduled().Local().Format("15:04:05"), p.Local().Format("15:04:05"), live, st.Boardable())
	}

	out := &bytes.Buffer{}
	a := &App{Client: c, Cache: &cache.Cache{Dir: t.TempDir(), TTL: time.Hour}, Out: out}
	if err := a.Run(ctx, Options{Route: "155", Query: "zugligeti", Count: 3}); err != nil {
		t.Fatalf("run: %v", err)
	}
	fmt.Print(out.String())
	if !strings.Contains(out.String(), "155 →") && !strings.Contains(out.String(), "No upcoming") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}
