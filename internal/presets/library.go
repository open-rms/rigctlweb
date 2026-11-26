package presets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL is the public openRMS Rigs API.
const DefaultBaseURL = "https://rigs.openrms.dev/api/v1"

// ProfileSchema is the only document format this package reads.
const ProfileSchema = "openrms-rigs/profile/v1"

// maxDump caps a downloaded library, so a broken or hostile server cannot fill the disk.
const maxDump = 16 << 20

var profileIDRe = regexp.MustCompile(`^[a-z0-9-]{1,120}$`)

// Library is the local copy of the openRMS Rigs profiles. It is fetched only when Update is
// called (the operator's "Update library" button) and kept on disk, so presets work offline.
type Library struct {
	path string
	base string
	ua   string
	hc   *http.Client

	mu     sync.RWMutex
	data   *cache
	byID   map[string]Envelope
	lastEr string
}

type cache struct {
	FetchedAt time.Time `json:"fetched_at"`
	ETag      string    `json:"etag,omitempty"`
	Source    string    `json:"source"`
	Dump      Dump      `json:"dump"`
}

// LibraryStatus is what the UI shows about the local copy.
type LibraryStatus struct {
	Available   bool      `json:"available"`
	FetchedAt   time.Time `json:"fetchedAt,omitzero"`
	GeneratedAt string    `json:"generatedAt,omitempty"`
	Source      string    `json:"source"`
	License     string    `json:"license,omitempty"`
	Count       int       `json:"count"`
	Error       string    `json:"error,omitempty"`
}

// CheckBaseURL accepts https URLs, and plain http only to a loopback address (for testing or
// a local mirror).
func CheckBaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("presets URL %q is not a plain http(s) URL", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if ip := net.ParseIP(u.Hostname()); (ip != nil && ip.IsLoopback()) || u.Hostname() == "localhost" {
			return nil
		}
	}
	return fmt.Errorf("presets URL %q must use https (http only to a loopback address)", raw)
}

// NewLibrary opens the library cached in dir (it may not exist yet). base is the Rigs API URL.
func NewLibrary(dir, base, userAgent string) (*Library, error) {
	base = strings.TrimRight(base, "/")
	if err := CheckBaseURL(base); err != nil {
		return nil, err
	}
	l := &Library{path: filepath.Join(dir, "presets-library.json"), base: base, ua: userAgent}
	u, _ := url.Parse(base)
	l.hc = &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Host != u.Host || req.URL.Scheme != u.Scheme || len(via) > 3 {
				return errors.New("redirect away from the presets server refused")
			}
			return nil
		},
	}
	b, err := os.ReadFile(l.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return l, err
	default:
		var c cache
		if err := json.Unmarshal(b, &c); err != nil {
			l.lastEr = "the saved library could not be read; update it again"
		} else {
			l.set(&c)
		}
	}
	return l, nil
}

func (l *Library) set(c *cache) {
	c.Dump.Profiles = usable(c.Dump.Profiles)
	by := make(map[string]Envelope, len(c.Dump.Profiles))
	for _, p := range c.Dump.Profiles {
		by[p.ID] = p
	}
	l.data, l.byID = c, by
}

// usable keeps published v1 documents with sane ids, sorted by radio then title.
func usable(in []Envelope) []Envelope {
	out := make([]Envelope, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		if p.Status != "published" || p.Document.Schema != ProfileSchema || !profileIDRe.MatchString(p.ID) || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Document, out[j].Document
		if a.Rig.Mfg+a.Rig.Name != b.Rig.Mfg+b.Rig.Name {
			return a.Rig.Mfg+a.Rig.Name < b.Rig.Mfg+b.Rig.Name
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// Status describes the local copy.
func (l *Library) Status() LibraryStatus {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s := LibraryStatus{Source: l.base, Error: l.lastEr}
	if l.data != nil {
		s.Available = true
		s.FetchedAt = l.data.FetchedAt
		s.GeneratedAt = l.data.Dump.GeneratedAt
		s.License = l.data.Dump.License
		s.Count = len(l.data.Dump.Profiles)
	}
	return s
}

// Profiles returns every usable profile in the local copy.
func (l *Library) Profiles() []Envelope {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.data == nil {
		return nil
	}
	return l.data.Dump.Profiles
}

// Get returns one profile from the local copy.
func (l *Library) Get(id string) (Envelope, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	p, ok := l.byID[id]
	return p, ok
}

// Update downloads the library. It sends nothing about the station: a plain GET with a
// User-Agent naming the program, and If-None-Match when a copy exists.
func (l *Library) Update(ctx context.Context) error {
	err := l.update(ctx)
	l.mu.Lock()
	l.lastEr = ""
	if err != nil {
		l.lastEr = err.Error()
	}
	l.mu.Unlock()
	return err
}

func (l *Library) update(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.base+"/dump.json", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", l.ua)
	l.mu.RLock()
	prev := l.data
	l.mu.RUnlock()
	if prev != nil && prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}
	resp, err := l.hc.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the presets server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified && prev != nil {
		c := *prev
		c.FetchedAt = time.Now().UTC()
		return l.save(&c)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the presets server answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDump+1))
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if len(body) > maxDump {
		return fmt.Errorf("the library is larger than %d MB; refused", maxDump>>20)
	}
	var d Dump
	if err := json.Unmarshal(body, &d); err != nil {
		return fmt.Errorf("the presets server sent something that is not a library: %w", err)
	}
	if d.Profiles == nil {
		return errors.New("the presets server sent a library without profiles")
	}
	return l.save(&cache{FetchedAt: time.Now().UTC(), ETag: resp.Header.Get("ETag"), Source: l.base, Dump: d})
}

func (l *Library) save(c *cache) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(l.path, b); err != nil {
		return fmt.Errorf("could not save the library: %w", err)
	}
	l.mu.Lock()
	l.set(c)
	l.mu.Unlock()
	return nil
}

func writeFileAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
