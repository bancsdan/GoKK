// Package cache is a tiny JSON-on-disk cache with a TTL, used for route
// reference data so only the live arrivals call hits the network.
package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cache stores JSON blobs under Dir; entries older than TTL are ignored.
type Cache struct {
	Dir string
	TTL time.Duration
	Now func() time.Time
}

// Default returns a cache in $XDG_CACHE_HOME/bkk, defaulting to ~/.cache/bkk.
func Default(ttl time.Duration) *Cache {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".cache")
		} else {
			dir = os.TempDir()
		}
	}
	return &Cache{Dir: filepath.Join(dir, "bkk"), TTL: ttl, Now: time.Now}
}

type entry struct {
	SavedAt int64           `json:"savedAt"`
	Data    json.RawMessage `json:"data"`
}

// Load unmarshals the entry named key into v and reports whether a fresh
// entry existed. Missing, expired or corrupt entries all report false.
func (c *Cache) Load(key string, v any) bool {
	if c == nil {
		return false
	}
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return false
	}
	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		return false
	}
	if c.now().Sub(time.Unix(e.SavedAt, 0)) > c.TTL {
		return false
	}
	return json.Unmarshal(e.Data, v) == nil
}

// Store writes v under key atomically.
func (c *Cache) Store(key string, v any) error {
	if c == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b, err := json.Marshal(entry{SavedAt: c.now().Unix(), Data: data})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		if werr != nil {
			return werr
		}
		return cerr
	}
	return os.Rename(tmp.Name(), c.path(key))
}

func (c *Cache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// path maps an arbitrary key to a safe file name.
func (c *Cache) path(key string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(key) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	sum := sha1.Sum([]byte(key))
	return filepath.Join(c.Dir, sb.String()+"-"+hex.EncodeToString(sum[:4])+".json")
}
