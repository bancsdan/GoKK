// Command bkk prints the next real-time departures of a Budapest (BKK)
// transit route at a fuzzy-matched stop.
//
//	bkk <route> <stop-query> [-c N] [-t] [-r]
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bancsdan/GoKK/internal/app"
	"github.com/bancsdan/GoKK/internal/cache"
	"github.com/bancsdan/GoKK/internal/futar"
)

const (
	cacheTTL = 24 * time.Hour
	timeout  = 10 * time.Second
	usage    = `usage: bkk <route> <stop-query> [-c N] [-t] [-r]
       bkk <route> -l

Print the next departures of a BKK route at a stop, grouped by direction.

  <route>       route short name as riders know it: 155, 4, M2, 9, 7E
  <stop-query>  free-text stop name; accent-insensitive and fuzzy
                ("viranyos" matches "Virányos út")

  -c, --count N   departures to show per direction (default 1)
  -t, --times     also print clock times, e.g. 5m42s (22:41)
  -r, --refresh   ignore the route/stop cache (~/.cache/bkk)
  -l, --list      list the route's stops by direction instead of arrivals
  -h, --help      show this help

Times are real-time predictions; ~ marks schedule-only entries.
Set BKK_API_KEY (free key: https://opendata.bkk.hu) or put the key in
$XDG_CONFIG_HOME/bkk/api_key (~/.config/bkk/api_key).`
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var ue *app.UsageError
		if errors.As(err, &ue) {
			fmt.Fprintln(os.Stderr, ue.Msg)
		} else {
			fmt.Fprintln(os.Stderr, "bkk:", err)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}
	if opts == nil { // help
		fmt.Println(usage)
		return nil
	}
	// The key is only required once a network call happens, so cached
	// lookups (e.g. -l on a known route) work without one.
	key, keyErr := apiKey()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	a := &app.App{
		Client: futar.New(key),
		Cache:  cache.Default(cacheTTL),
		Out:    os.Stdout,
		Color:  colorEnabled(),
	}
	err = a.Run(ctx, *opts)
	if errors.Is(err, futar.ErrNoKey) {
		return keyErr
	}
	return err
}

// parseArgs accepts flags anywhere among the positionals so that
// "bkk 155 zugligeti -c 3" works. A nil result means help was requested.
func parseArgs(args []string) (*app.Options, error) {
	o := &app.Options{Count: 1}
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, hasVal := strings.Cut(arg, "=")
		switch name {
		case "-h", "--help":
			return nil, nil
		case "-t", "--times":
			o.ShowClock = true
		case "-r", "--refresh":
			o.Refresh = true
		case "-l", "--list":
			o.List = true
		case "-c", "--count":
			if !hasVal {
				if i+1 >= len(args) {
					return nil, &app.UsageError{Msg: "-c needs a number\n" + usage}
				}
				i++
				val = args[i]
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, &app.UsageError{Msg: fmt.Sprintf("-c: %q is not a positive number", val)}
			}
			o.Count = n
		default:
			if strings.HasPrefix(arg, "-") && len(arg) > 1 {
				return nil, &app.UsageError{Msg: fmt.Sprintf("unknown flag %s\n%s", arg, usage)}
			}
			pos = append(pos, arg)
		}
	}
	if len(pos) == 0 || (len(pos) < 2 && !o.List) {
		return nil, &app.UsageError{Msg: usage}
	}
	o.Route = pos[0]
	o.Query = strings.Join(pos[1:], " ")
	return o, nil
}

// apiKey reads BKK_API_KEY, falling back to a one-line config file.
func apiKey() (string, error) {
	if k := strings.TrimSpace(os.Getenv("BKK_API_KEY")); k != "" {
		return k, nil
	}
	path := keyFile()
	if b, err := os.ReadFile(path); err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, nil
		}
	}
	return "", fmt.Errorf("no API key. Get a free key at https://opendata.bkk.hu, then\n"+
		"  export BKK_API_KEY=<key>   or write it to %s", path)
}

// keyFile is $XDG_CONFIG_HOME/bkk/api_key, defaulting to ~/.config/bkk/api_key.
func keyFile() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "~/.config/bkk/api_key"
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "bkk", "api_key")
}

// colorEnabled is true when stdout is a TTY and NO_COLOR is not set.
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
