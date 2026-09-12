package cache

import (
	"testing"
	"time"
)

func TestRoundTripAndTTL(t *testing.T) {
	now := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
	c := &Cache{Dir: t.TempDir(), TTL: time.Hour, Now: func() time.Time { return now }}

	var got []string
	if c.Load("routes/155", &got) {
		t.Fatal("expected miss on empty cache")
	}
	if err := c.Store("routes/155", []string{"BKK_1550"}); err != nil {
		t.Fatal(err)
	}
	if !c.Load("routes/155", &got) || len(got) != 1 || got[0] != "BKK_1550" {
		t.Fatalf("load = %v", got)
	}
	now = now.Add(2 * time.Hour)
	if c.Load("routes/155", &got) {
		t.Fatal("expected expired entry to miss")
	}
}

func TestKeysAreSafeAndDistinct(t *testing.T) {
	c := &Cache{Dir: "/x", TTL: time.Hour}
	a, b := c.path("route/M2"), c.path("route/m2")
	if a == b {
		t.Fatal("distinct keys must map to distinct files")
	}
	if c.path("../etc") != c.path("../etc") {
		t.Fatal("path must be deterministic")
	}
	for _, p := range []string{a, b, c.path("../../etc/passwd")} {
		if got := p[len("/x/"):]; got == "" || got[0] == '.' {
			t.Fatalf("unsafe file name %q", got)
		}
	}
}
