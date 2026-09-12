package main

import (
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
		{[]string{"155", "zugligeti"}, &app.Options{Route: "155", Query: "zugligeti", Count: 1}, false},
		{[]string{"155", "zugligeti", "-c", "3"}, &app.Options{Route: "155", Query: "zugligeti", Count: 3}, false},
		{[]string{"-c=2", "M2", "deak", "ferenc", "-t", "-r"}, &app.Options{Route: "M2", Query: "deak ferenc", Count: 2, ShowClock: true, Refresh: true}, false},
		{[]string{"--count", "4", "4", "moricz"}, &app.Options{Route: "4", Query: "moricz", Count: 4}, false},
		{[]string{"155", "-l"}, &app.Options{Route: "155", Count: 1, List: true}, false},
		{[]string{"--list", "155", "x"}, &app.Options{Route: "155", Query: "x", Count: 1, List: true}, false},
		{[]string{"-h"}, nil, false},
		{[]string{"-l"}, nil, true},
		{[]string{"155"}, nil, true},
		{[]string{"155", "x", "-c", "0"}, nil, true},
		{[]string{"155", "x", "-c"}, nil, true},
		{[]string{"155", "x", "--bogus"}, nil, true},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args)
		if (err != nil) != c.err {
			t.Errorf("%v: err = %v, wantErr %v", c.args, err, c.err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %+v, want %+v", c.args, got, c.want)
		}
	}
}
