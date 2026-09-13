package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bancsdan/GoKK/internal/app"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want *app.Options
		err  bool
	}{
		{[]string{"155", "viranyos"}, &app.Options{Route: "155", Query: "viranyos", Count: 1}, false},
		{[]string{"155", "viranyos", "-c", "3"}, &app.Options{Route: "155", Query: "viranyos", Count: 3}, false},
		{[]string{"-c=2", "M2", "deak", "ferenc", "-t", "-r", "-j"}, &app.Options{Route: "M2", Query: "deak ferenc", Count: 2, ShowClock: true, Refresh: true, JSON: true}, false},
		{[]string{"--count", "4", "4", "moricz"}, &app.Options{Route: "4", Query: "moricz", Count: 4}, false},
		{[]string{"155", "-l"}, &app.Options{Route: "155", Count: 1, List: true}, false},
		{[]string{"--list", "155", "x", "--json"}, &app.Options{Route: "155", Query: "x", Count: 1, List: true, JSON: true}, false},
		{[]string{"-h"}, nil, false},
		{[]string{}, nil, true},
		{[]string{"-l"}, nil, true},
		{[]string{"155"}, nil, true},
		{[]string{"155", "x", "-c", "0"}, nil, true},
		{[]string{"155", "x", "-c"}, nil, true},
		{[]string{"155", "x", "--bogus"}, nil, true},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args, nil)
		if (err != nil) != c.err {
			t.Errorf("%v: err = %v, wantErr %v", c.args, err, c.err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.args, got, c.want)
		}
	}
}

func TestParseArgsAliases(t *testing.T) {
	aliases := map[string]string{"home": "155 viranyos -c 3", "work": "4 moricz", "default": "home"}
	cases := []struct {
		args []string
		want *app.Options
	}{
		{[]string{"home"}, &app.Options{Route: "155", Query: "viranyos", Count: 3}},
		{[]string{"home", "-c", "1", "-t"}, &app.Options{Route: "155", Query: "viranyos", Count: 1, ShowClock: true}}, // later flags win
		{[]string{"-j", "work"}, &app.Options{Route: "4", Query: "moricz", Count: 1, JSON: true}},
		{[]string{}, &app.Options{Route: "155", Query: "viranyos", Count: 3}}, // default → home
		{[]string{"-t"}, &app.Options{Route: "155", Query: "viranyos", Count: 3, ShowClock: true}},
		{[]string{"work", "-l"}, &app.Options{Route: "4", Query: "moricz", Count: 1, List: true}},
		{[]string{"155", "home"}, &app.Options{Route: "155", Query: "home", Count: 1}}, // only the first positional expands
	}
	for _, c := range cases {
		got, err := parseArgs(c.args, aliases)
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.args, got, c.want)
		}
	}
}

func TestLoadAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if got, err := loadAliases(path); err != nil || got != nil {
		t.Fatalf("missing file: %v %v", got, err)
	}
	if err := os.WriteFile(path, []byte("# comment\n\nhome = 155 viranyos -c 3\n  work=4 moricz  \ndefault = home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadAliases(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"home": "155 viranyos -c 3", "work": "4 moricz", "default": "home"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if err := os.WriteFile(path, []byte("home 155\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAliases(path); err == nil {
		t.Fatal("malformed line should error")
	}
}
