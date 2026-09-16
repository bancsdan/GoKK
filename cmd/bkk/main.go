// Command bkk prints the next real-time departures of a Budapest (BKK)
// transit route at a fuzzy-matched stop.
//
//	bkk <route> <stop-query> [-c N] [-t] [-j] [-r] [-s NAME]
//	bkk <route> -l
//	bkk <alias> [flags]
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	usage    = `usage: bkk <route> <stop-query> [-c N] [-t] [-j] [-r] [-s NAME]
       bkk <route> -l [-j]
       bkk <alias> [flags]

Print the next departures of a BKK route at a stop, grouped by direction.

  <route>       route short name as riders know it: 155, 4, M2, 9, 7E
  <stop-query>  free-text stop name; accent-insensitive and fuzzy
                ("viranyos" matches "Virányos út")
  <alias>       a name from the config file (see -a and -s)

  -c, --count N     departures to show per direction (default 1)
  -t, --times       also print clock times, e.g. 5m42s (22:41)
  -j, --json        print JSON instead of text
  -l, --list        list the route's stops by direction instead of arrivals
  -r, --refresh     ignore the route/stop cache (~/.cache/bkk)
  -s, --save NAME   run, then save this command as alias NAME
  -a, --aliases     show the configured aliases and exit
  -h, --help        show this help

Times are real-time predictions; ~ marks schedule-only entries.
Set BKK_API_KEY (free key: https://opendata.bkk.hu) or put the key in
$XDG_CONFIG_HOME/bkk/api_key (~/.config/bkk/api_key).

Aliases live in $XDG_CONFIG_HOME/bkk/config (~/.config/bkk/config), one
per line as "name = args"; "default" is used when bkk runs with no args:

  home    = 155 viranyos -c 3
  work    = 4 moricz
  default = home

"bkk 155 viranyos -c 3 --save home" writes the first line for you.`
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
	aliases, aliasErr := loadAliases(configFile())
	if aliasErr != nil {
		return aliasErr
	}
	args, saveAs, err := splitSave(args)
	if err != nil {
		return err
	}
	opts, expanded, err := parseArgs(args, aliases)
	if err != nil {
		return err
	}
	if opts == nil { // help or alias listing already printed
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
		Color:  colorEnabled() && !opts.JSON,
	}
	err = a.Run(ctx, *opts)
	if errors.Is(err, futar.ErrNoKey) {
		return keyErr
	}
	if err != nil || saveAs == "" {
		return err
	}
	// Only a command that worked is worth remembering, so a typo in the
	// stop name or an unknown route never ends up in the config.
	val := strings.Join(expanded, " ")
	path := configFile()
	if err := saveAlias(path, saveAs, val); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "saved alias %s = %s (%s)\n", saveAs, val, tildePath(path))
	return nil
}

// tildePath shortens a path under the home directory to ~/... for display.
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

// splitSave removes "-s NAME", "--save NAME" and "--save=NAME" from args
// and returns the alias name, or "" when the flag is absent. It runs before
// alias expansion so the flag is never mistaken for part of a command.
func splitSave(args []string) (rest []string, name string, err error) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		flag, val, hasVal := strings.Cut(args[i], "=")
		if flag != "-s" && flag != "--save" {
			rest = append(rest, args[i])
			continue
		}
		if !hasVal {
			if i+1 >= len(args) {
				return nil, "", &app.UsageError{Msg: flag + " needs an alias name\n" + usage}
			}
			i++
			val = args[i]
		}
		if val == "" || strings.HasPrefix(val, "-") || strings.ContainsAny(val, " \t=") {
			return nil, "", &app.UsageError{Msg: fmt.Sprintf("%s: %q is not a valid alias name", flag, val)}
		}
		name = val
	}
	return rest, name, nil
}

// parseArgs accepts flags anywhere among the positionals so that
// "bkk 155 viranyos -c 3" works. If the first positional is an alias it is
// replaced by the alias's arguments; with no positionals at all the
// "default" alias is used. The second result is the argument list after
// alias expansion, which is what --save records. A nil Options means help
// or -a was handled.
func parseArgs(args []string, aliases map[string]string) (*app.Options, []string, error) {
	o, pos, err := parseOnce(args)
	if err != nil || o == nil {
		return o, nil, err
	}
	expanded := args
	switch {
	case len(pos) > 0 && aliases[pos[0]] != "":
		expanded = append(strings.Fields(aliases[pos[0]]), removeFirst(args, pos[0])...)
		o, pos, err = parseOnce(expanded)
	case len(pos) == 0 && aliases["default"] != "":
		def := aliases["default"]
		if aliases[def] != "" { // "default = home"
			def = aliases[def]
		}
		expanded = append(strings.Fields(def), args...)
		o, pos, err = parseOnce(expanded)
	}
	if err != nil || o == nil {
		return o, nil, err
	}
	if len(pos) == 0 || (len(pos) < 2 && !o.List) {
		return nil, nil, &app.UsageError{Msg: usage}
	}
	o.Route = pos[0]
	o.Query = strings.Join(pos[1:], " ")
	return o, expanded, nil
}

// parseOnce tokenizes one argument list into options and positionals
// without validating the positional count.
func parseOnce(args []string) (*app.Options, []string, error) {
	o := &app.Options{Count: 1}
	var pos []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, val, hasVal := strings.Cut(arg, "=")
		switch name {
		case "-h", "--help":
			fmt.Println(usage)
			return nil, nil, nil
		case "-a", "--aliases":
			printAliases()
			return nil, nil, nil
		case "-t", "--times":
			o.ShowClock = true
		case "-j", "--json":
			o.JSON = true
		case "-r", "--refresh":
			o.Refresh = true
		case "-l", "--list":
			o.List = true
		case "-c", "--count":
			if !hasVal {
				if i+1 >= len(args) {
					return nil, nil, &app.UsageError{Msg: "-c needs a number\n" + usage}
				}
				i++
				val = args[i]
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return nil, nil, &app.UsageError{Msg: fmt.Sprintf("-c: %q is not a positive number", val)}
			}
			o.Count = n
		default:
			if strings.HasPrefix(arg, "-") && len(arg) > 1 {
				return nil, nil, &app.UsageError{Msg: fmt.Sprintf("unknown flag %s\n%s", arg, usage)}
			}
			pos = append(pos, arg)
		}
	}
	return o, pos, nil
}

func removeFirst(args []string, tok string) []string {
	out := make([]string, 0, len(args))
	removed := false
	for _, a := range args {
		if !removed && a == tok {
			removed = true
			continue
		}
		out = append(out, a)
	}
	return out
}

// loadAliases reads "name = args" lines; missing file means no aliases.
func loadAliases(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("config %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	aliases := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, val, ok := strings.Cut(line, "=")
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		if !ok || name == "" || val == "" || strings.ContainsAny(name, " \t") {
			return nil, &app.UsageError{Msg: fmt.Sprintf("%s:%d: expected \"name = args\", got %q", path, n, line)}
		}
		aliases[name] = val
	}
	return aliases, sc.Err()
}

// saveAlias writes "name = val" to the config file, replacing an existing
// line for name in place and otherwise appending. Comments, blank lines and
// other aliases are kept as they are; a missing file or directory is created.
func saveAlias(path, name, val string) error {
	var lines []string
	if b, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		if len(lines) == 1 && lines[0] == "" {
			lines = nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("config %s: %v", path, err)
	}
	entry := name + " = " + val
	replaced := false
	for i, line := range lines {
		n, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && !strings.HasPrefix(strings.TrimSpace(line), "#") && strings.TrimSpace(n) == name {
			lines[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, entry)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return fmt.Errorf("config %s: %v", path, err)
	}
	return nil
}

func printAliases() {
	path := configFile()
	aliases, err := loadAliases(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	if len(aliases) == 0 {
		fmt.Printf("no aliases; run a lookup with --save NAME, or add lines like \"home = 155 viranyos -c 3\" to %s\n", path)
		return
	}
	names := make([]string, 0, len(aliases))
	width := 0
	for n := range aliases {
		names = append(names, n)
		if len(n) > width {
			width = len(n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Printf("%-*s = %s\n", width, n, aliases[n])
	}
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

// configDir is $XDG_CONFIG_HOME/bkk, defaulting to ~/.config/bkk.
func configDir() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "~/.config/bkk"
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "bkk")
}

func keyFile() string    { return filepath.Join(configDir(), "api_key") }
func configFile() string { return filepath.Join(configDir(), "config") }

// colorEnabled is true when stdout is a TTY and NO_COLOR is not set.
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
