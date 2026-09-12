// Package futar is a thin client for the BKK FUTÁR OpenData API
// (https://opendata.bkk.hu/docs/futar-openapi.yaml). Only the three
// endpoints bkk needs are wrapped: search, route-details and
// arrivals-and-departures-for-stop.
package futar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultBaseURL is the OpenAPI server URL joined with the "otp" dialect and
// the common "/api/where" prefix shared by every endpoint in the spec.
const DefaultBaseURL = "https://futar.bkk.hu/api/query/v1/ws/otp/api/where"

// ErrUnauthorized is returned when the API rejects the key (HTTP 401).
var ErrUnauthorized = errors.New("API key rejected by BKK (HTTP 401)")

// ErrNoKey is returned when a request is attempted with an empty key.
var ErrNoKey = errors.New("missing API key")

// ErrNotFound is returned when the API reports HTTP 404 (unknown route ID).
var ErrNotFound = errors.New("not found")

// Client talks to the FUTÁR API. The zero value is not usable; use New.
type Client struct {
	Key     string
	BaseURL string
	HTTP    *http.Client
}

// New returns a client authenticating with key.
func New(key string) *Client {
	return &Client{Key: key, BaseURL: DefaultBaseURL, HTTP: &http.Client{}}
}

// Route is a transit route (TransitRoute in the spec).
type Route struct {
	ID          string `json:"id"`
	ShortName   string `json:"shortName"`
	LongName    string `json:"longName,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
}

// Stop is a stop/platform (TransitStop in the spec).
type Stop struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Variant is one pattern of a route (TransitRouteVariant): a direction, its
// headsign and the ordered stop IDs it serves.
type Variant struct {
	Direction string   `json:"direction"`
	Headsign  string   `json:"headsign"`
	StopIDs   []string `json:"stopIds"`
}

// RouteDetails is a route with its variants and the stops they reference.
type RouteDetails struct {
	Route
	Variants []Variant       `json:"variants"`
	Stops    map[string]Stop `json:"stops"`
}

// Trip is a scheduled trip (TransitTrip).
type Trip struct {
	ID          string `json:"id"`
	RouteID     string `json:"routeId"`
	DirectionID string `json:"directionId"`
	Headsign    string `json:"tripHeadsign"`
}

// StopTime is one arrival/departure at a stop (TransitScheduleStopTime).
// Times are epoch seconds; zero means absent.
type StopTime struct {
	StopID                 string `json:"stopId"`
	TripID                 string `json:"tripId"`
	StopHeadsign           string `json:"stopHeadsign"`
	ArrivalTime            int64  `json:"arrivalTime"`
	DepartureTime          int64  `json:"departureTime"`
	PredictedArrivalTime   int64  `json:"predictedArrivalTime"`
	PredictedDepartureTime int64  `json:"predictedDepartureTime"`
	PredictionScheduled    bool   `json:"predictionScheduled"`
	PickupAllowed          *bool  `json:"pickupAllowed"`
	Uncertain              bool   `json:"uncertain"`
}

// Arrivals is the arrivals-and-departures-for-stop result.
type Arrivals struct {
	CurrentTime time.Time
	StopTimes   []StopTime
	Trips       map[string]Trip
	Stops       map[string]Stop
}

// Scheduled returns the scheduled departure (or arrival) time.
func (s StopTime) Scheduled() time.Time { return toTime(first(s.DepartureTime, s.ArrivalTime)) }

// Predicted returns the real-time predicted departure (or arrival) time and
// whether a live prediction exists. A prediction flagged predictionScheduled
// is schedule-derived, so it does not count as live.
func (s StopTime) Predicted() (time.Time, bool) {
	v := first(s.PredictedDepartureTime, s.PredictedArrivalTime)
	if v == 0 || s.PredictionScheduled {
		return time.Time{}, false
	}
	return toTime(v), true
}

// Boardable reports whether riders may board here (pickupAllowed != false).
func (s StopTime) Boardable() bool { return s.PickupAllowed == nil || *s.PickupAllowed }

// SearchRoutes queries the search endpoint and returns the routes it
// references. Filtering by short name is left to the caller.
func (c *Client) SearchRoutes(ctx context.Context, query string) ([]Route, error) {
	var data struct {
		Entry struct {
			RouteIDs []string `json:"routeIds"`
		} `json:"entry"`
		References struct {
			Routes map[string]Route `json:"routes"`
		} `json:"references"`
	}
	q := url.Values{"query": {query}, "includeReferences": {"routes"}}
	if err := c.get(ctx, "search", q, &data); err != nil {
		return nil, err
	}
	routes := make([]Route, 0, len(data.Entry.RouteIDs))
	for _, id := range data.Entry.RouteIDs {
		if r, ok := data.References.Routes[id]; ok {
			routes = append(routes, r)
		}
	}
	return routes, nil
}

// RouteDetails fetches a route with its variants and stop references.
func (c *Client) RouteDetails(ctx context.Context, routeID string) (*RouteDetails, error) {
	var data struct {
		Entry struct {
			Route
			Variants []Variant `json:"variants"`
		} `json:"entry"`
		References struct {
			Stops map[string]Stop `json:"stops"`
		} `json:"references"`
	}
	q := url.Values{"routeId": {routeID}, "includeReferences": {"stops"}}
	if err := c.get(ctx, "route-details", q, &data); err != nil {
		return nil, err
	}
	rd := &RouteDetails{Route: data.Entry.Route, Variants: data.Entry.Variants, Stops: map[string]Stop{}}
	for _, v := range rd.Variants {
		for _, id := range v.StopIDs {
			if s, ok := data.References.Stops[id]; ok {
				rd.Stops[id] = s
			}
		}
	}
	return rd, nil
}

// Arrivals fetches upcoming stop times at stopIDs within the next `after`
// window. routeIDs (may be empty) asks the server to make sure those routes
// are represented; callers should still filter by route.
func (c *Client) Arrivals(ctx context.Context, stopIDs, routeIDs []string, after time.Duration) (*Arrivals, error) {
	var data struct {
		Entry struct {
			StopTimes []StopTime `json:"stopTimes"`
		} `json:"entry"`
		References struct {
			Trips map[string]Trip `json:"trips"`
			Stops map[string]Stop `json:"stops"`
		} `json:"references"`
	}
	// stopId/includeRouteId are OpenAPI arrays without explode:false, so
	// they go as repeated params; includeReferences is explode:false.
	q := url.Values{
		"stopId":            stopIDs,
		"minutesBefore":     {"0"},
		"minutesAfter":      {fmt.Sprint(int(after.Minutes()))},
		"limit":             {"200"},
		"onlyDepartures":    {"false"},
		"includeReferences": {"trips,stops"},
	}
	if len(routeIDs) > 0 {
		q["includeRouteId"] = routeIDs
	}
	now, err := c.getEnvelope(ctx, "arrivals-and-departures-for-stop", q, &data)
	if err != nil {
		return nil, err
	}
	a := &Arrivals{CurrentTime: now, StopTimes: data.Entry.StopTimes, Trips: data.References.Trips, Stops: data.References.Stops}
	if a.Trips == nil {
		a.Trips = map[string]Trip{}
	}
	return a, nil
}

type envelope struct {
	Code        int             `json:"code"`
	CurrentTime int64           `json:"currentTime"`
	Status      string          `json:"status"`
	Text        string          `json:"text"`
	Data        json.RawMessage `json:"data"`
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	_, err := c.getEnvelope(ctx, path, q, out)
	return err
}

// getEnvelope performs a GET, unwraps the OneBusAway-style envelope into out
// and returns the server's current time.
func (c *Client) getEnvelope(ctx context.Context, path string, q url.Values, out any) (time.Time, error) {
	if c.Key == "" {
		return time.Time{}, ErrNoKey
	}
	q.Set("key", c.Key)
	q.Set("version", "4")
	q.Set("appVersion", "bkk-cli")
	u := strings.TrimRight(c.BaseURL, "/") + "/" + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return time.Time{}, err
	}
	req.Header.Set("Accept", "application/json")
	debug := os.Getenv("BKK_DEBUG") != ""
	if debug {
		fmt.Fprintln(os.Stderr, "GET", strings.Replace(u, "key="+url.QueryEscape(c.Key), "key=***", 1))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return time.Time{}, netErr(err)
	}
	defer resp.Body.Close()
	if debug {
		fmt.Fprintln(os.Stderr, "   ", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return time.Time{}, netErr(err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return time.Time{}, ErrUnauthorized
	case http.StatusNotFound:
		return time.Time{}, fmt.Errorf("futar %s: %w (HTTP 404): %s", path, ErrNotFound, trim(body))
	default:
		return time.Time{}, fmt.Errorf("futar %s: HTTP %d: %s", path, resp.StatusCode, trim(body))
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return time.Time{}, fmt.Errorf("futar %s: bad JSON: %v", path, err)
	}
	if env.Code != 0 && env.Code != 200 {
		if env.Code == 404 {
			return time.Time{}, fmt.Errorf("futar %s: %w: %s", path, ErrNotFound, env.Text)
		}
		return time.Time{}, fmt.Errorf("futar %s: %d %s", path, env.Code, env.Text)
	}
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return time.Time{}, fmt.Errorf("futar %s: bad JSON: %v", path, err)
		}
	}
	now := toTime(env.CurrentTime)
	if now.IsZero() {
		now = time.Now()
	}
	return now, nil
}

func netErr(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timed out talking to futar.bkk.hu")
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return errors.New("timed out talking to futar.bkk.hu")
	}
	return fmt.Errorf("network error: %v", err)
}

func trim(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

func first(vals ...int64) int64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

// toTime converts an epoch value that may be in seconds or milliseconds.
func toTime(v int64) time.Time {
	switch {
	case v == 0:
		return time.Time{}
	case v > 1e12:
		return time.UnixMilli(v)
	default:
		return time.Unix(v, 0)
	}
}
