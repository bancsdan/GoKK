package match

import (
	"reflect"
	"testing"
)

var stops = []Stop{
	{"BKK_F02001", "Zugligeti út"},
	{"BKK_F02002", "Zugligeti út"},
	{"BKK_F02003", "Széll Kálmán tér M"},
	{"BKK_F02004", "Széll Kálmán tér M"},
	{"BKK_F02005", "Zugliget, Libegő"},
	{"BKK_F02006", "Városmajor"},
	{"BKK_F02007", "Nyúl utca"},
	{"BKK_F02008", "Kútvölgyi út"},
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Zugligeti út":        "zugligeti ut",
		"Széll Kálmán tér M":  "szell kalman ter m",
		"  Őrmező,  Ürömi/x ": "ormezo uromi x",
		"ÁÉÍÓÖŐÚÜŰ":           "aeiooouuu",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindCollectsAllPlatforms(t *testing.T) {
	got := Find("zugligeti", stops)
	want := []Candidate{{Name: "Zugligeti út", IDs: []string{"BKK_F02001", "BKK_F02002"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Find = %+v, want %+v", got, want)
	}
}

func TestFindTiers(t *testing.T) {
	// Exact beats substring: "zugliget" is a substring of "zugligeti ut"
	// but there is no exact "zugliget" stop, so both substring hits show.
	if got := Find("zugliget", stops); len(got) != 2 {
		t.Fatalf("substring: got %d candidates, want 2 (ambiguous): %+v", len(got), got)
	}
	// Multi-word ASCII query against accented name.
	if got := Find("szell kalman", stops); len(got) != 1 || got[0].Name != "Széll Kálmán tér M" {
		t.Fatalf("substring multi-word: %+v", got)
	}
	// Typo falls back to edit distance.
	if got := Find("zugligeti utt", stops); len(got) != 1 || got[0].Name != "Zugligeti út" {
		t.Fatalf("fuzzy: %+v", got)
	}
	if got := Find("varosmajr", stops); len(got) != 1 || got[0].Name != "Városmajor" {
		t.Fatalf("fuzzy single word: %+v", got)
	}
	// Garbage matches nothing.
	if got := Find("xyzxyzxyz", stops); len(got) != 0 {
		t.Fatalf("none: %+v", got)
	}
	// Short query with a typo should not match wildly.
	if got := Find("nyal", stops); len(got) != 1 || got[0].Name != "Nyúl utca" {
		t.Fatalf("short fuzzy: %+v", got)
	}
}

func TestClosest(t *testing.T) {
	got := Closest("kutvolgy", stops, 2)
	if len(got) != 2 || got[0].Name != "Kútvölgyi út" {
		t.Fatalf("Closest = %+v", got)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		d    int
	}{
		{"", "", 0}, {"a", "", 1}, {"kitten", "sitting", 3}, {"őrs", "ors", 1}, {"abc", "abc", 0},
	}
	for _, c := range cases {
		if got := Levenshtein(c.a, c.b); got != c.d {
			t.Errorf("Levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.d)
		}
	}
}
