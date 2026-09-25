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
		{[]string{"h5", "aquincum", "-H", "szentendre", "--heading=bekas"}, &app.Options{Route: "h5", Query: "aquincum", Count: 1, Headings: []string{"szentendre", "bekas"}}, false},
		{[]string{"h5", "aquincum", "-X", "batthyany ter", "--not-heading", "x"}, &app.Options{Route: "h5", Query: "aquincum", Count: 1, Excludes: []string{"batthyany ter", "x"}}, false},
		{[]string{"h5", "aquincum", "-H"}, nil, true},
		{[]string{"h5", "aquincum", "-X="}, nil, true},
		{[]string{"-h"}, nil, false},
		{[]string{}, nil, true},
		{[]string{"-l"}, nil, true},
		{[]string{"155"}, nil, true},
		{[]string{"155", "x", "-c", "0"}, nil, true},
		{[]string{"155", "x", "-c"}, nil, true},
		{[]string{"155", "x", "--bogus"}, nil, true},
	}
	for _, c := range cases {
		got, expanded, err := parseArgs(c.args, nil)
		if (err != nil) != c.err {
			t.Errorf("%v: err = %v, wantErr %v", c.args, err, c.err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.args, got, c.want)
		}
		if got != nil && !reflect.DeepEqual(expanded, c.args) {
			t.Errorf("%v: expanded = %v without aliases, want the input", c.args, expanded)
		}
	}
}

func TestParseArgsAliases(t *testing.T) {
	aliases := map[string]string{"home": "155 viranyos -c 3", "work": "4 moricz", "default": "home", "hev": `h5 aquincum -X "batthyany ter"`}
	cases := []struct {
		args     []string
		want     *app.Options
		expanded []string
	}{
		{[]string{"home"}, &app.Options{Route: "155", Query: "viranyos", Count: 3}, []string{"155", "viranyos", "-c", "3"}},
		{[]string{"home", "-c", "1", "-t"}, &app.Options{Route: "155", Query: "viranyos", Count: 1, ShowClock: true}, []string{"155", "viranyos", "-c", "3", "-c", "1", "-t"}}, // later flags win
		{[]string{"-j", "work"}, &app.Options{Route: "4", Query: "moricz", Count: 1, JSON: true}, []string{"4", "moricz", "-j"}},
		{[]string{}, &app.Options{Route: "155", Query: "viranyos", Count: 3}, []string{"155", "viranyos", "-c", "3"}}, // default → home
		{[]string{"-t"}, &app.Options{Route: "155", Query: "viranyos", Count: 3, ShowClock: true}, []string{"155", "viranyos", "-c", "3", "-t"}},
		{[]string{"work", "-l"}, &app.Options{Route: "4", Query: "moricz", Count: 1, List: true}, []string{"4", "moricz", "-l"}},
		{[]string{"155", "home"}, &app.Options{Route: "155", Query: "home", Count: 1}, []string{"155", "home"}}, // only the first positional expands
		{[]string{"hev"}, &app.Options{Route: "h5", Query: "aquincum", Count: 1, Excludes: []string{"batthyany ter"}}, []string{"h5", "aquincum", "-X", "batthyany ter"}},
	}
	for _, c := range cases {
		got, expanded, err := parseArgs(c.args, aliases)
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.args, got, c.want)
		}
		if !reflect.DeepEqual(expanded, c.expanded) {
			t.Errorf("%v: expanded = %v, want %v", c.args, expanded, c.expanded)
		}
	}
}

func TestJoinSplitArgs(t *testing.T) {
	args := []string{"h5", "aquincum", "-X", "batthyany ter", "--heading=pomaz x", "-t"}
	joined := joinArgs(args)
	if want := `h5 aquincum -X "batthyany ter" "--heading=pomaz x" -t`; joined != want {
		t.Errorf("joinArgs = %q, want %q", joined, want)
	}
	if got := splitArgs(joined); !reflect.DeepEqual(got, args) {
		t.Errorf("splitArgs(%q) = %q, want %q", joined, got, args)
	}
	if got := splitArgs("  155\tviranyos   -c 3 "); !reflect.DeepEqual(got, []string{"155", "viranyos", "-c", "3"}) {
		t.Errorf("splitArgs whitespace = %q", got)
	}
}

func TestSplitSave(t *testing.T) {
	cases := []struct {
		args []string
		rest []string
		name string
		err  bool
	}{
		{[]string{"155", "viranyos", "-c", "3"}, []string{"155", "viranyos", "-c", "3"}, "", false},
		{[]string{"155", "viranyos", "-c", "3", "-t", "--save", "home"}, []string{"155", "viranyos", "-c", "3", "-t"}, "home", false},
		{[]string{"-s", "home", "155", "viranyos"}, []string{"155", "viranyos"}, "home", false},
		{[]string{"155", "--save=work", "moricz"}, []string{"155", "moricz"}, "work", false},
		{[]string{"155", "-l", "-s", "stops"}, []string{"155", "-l"}, "stops", false},
		{[]string{"155", "viranyos", "--save"}, nil, "", true},
		{[]string{"155", "viranyos", "--save", "-t"}, nil, "", true},
		{[]string{"155", "viranyos", "--save="}, nil, "", true},
		{[]string{"155", "viranyos", "--save", "my home"}, nil, "", true},
		{[]string{"155", "viranyos", "--save", "a=b"}, nil, "", true},
	}
	for _, c := range cases {
		rest, name, err := splitSave(c.args)
		if (err != nil) != c.err {
			t.Errorf("%v: err = %v, wantErr %v", c.args, err, c.err)
			continue
		}
		if err != nil {
			continue
		}
		if !reflect.DeepEqual(rest, c.rest) || name != c.name {
			t.Errorf("%v: got %v %q, want %v %q", c.args, rest, name, c.rest, c.name)
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

func TestSaveAlias(t *testing.T) {
	// Missing directory and file: both are created.
	path := filepath.Join(t.TempDir(), "bkk", "config")
	if err := saveAlias(path, "home", "155 viranyos -c 3"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "home = 155 viranyos -c 3\n" {
		t.Fatalf("new file: %q", b)
	}

	// Existing file: append a new name, keep everything else verbatim.
	if err := saveAlias(path, "work", "4 moricz"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "home = 155 viranyos -c 3\nwork = 4 moricz\n" {
		t.Fatalf("append: %q", b)
	}

	// Existing name: replaced in place; comments and spacing survive.
	if err := os.WriteFile(path, []byte("# mine\n\n  home=155 viranyos\nwork = 4 moricz\ndefault = home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := saveAlias(path, "home", "155 viranyos -c 3 -t"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "# mine\n\nhome = 155 viranyos -c 3 -t\nwork = 4 moricz\ndefault = home\n" {
		t.Fatalf("replace: %q", b)
	}

	// What was written must read back through loadAliases.
	got, err := loadAliases(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"home": "155 viranyos -c 3 -t", "work": "4 moricz", "default": "home"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: %v", got)
	}
}
