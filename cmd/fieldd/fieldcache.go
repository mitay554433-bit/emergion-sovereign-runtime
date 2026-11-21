package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"emergion-sovereign-runtime/internal/proj"
	"emergion-sovereign-runtime/pkg/fieldapi"
)

// renderField builds the FIELD projection into dir (field.json, field.html,
// projection.current.json and accepted deliverables). Tests replace it.
var renderField = func(rt *fieldapi.Runtime, dir string) (proj.Receipt, error) {
	return rt.RenderCurrent(dir)
}

// fieldFreshWait is how long a /field.json request waits for a rebuild after
// the ledger changed before it answers with the last good snapshot instead.
func fieldFreshWait() time.Duration {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FIELDD_FIELD_WAIT_MS"))); err == nil && v >= 0 {
		return time.Duration(v) * time.Millisecond
	}
	return 1500 * time.Millisecond
}

// fieldSnapshot is one built /field.json body and where it came from.
type fieldSnapshot struct {
	body      []byte
	tip       string
	stamp     string // ledger stamp the build started from; "" = loaded from disk
	generated time.Time
	took      time.Duration
}

// fieldCache keeps /field.json off the request path. The projection is
// rebuilt by one background build at a time when the ledger changes (the
// ledger stamp moves); requests get the snapshot that matches the ledger, or
// after a short wait the last good snapshot, marked stale by headers. The
// body is byte-for-byte what RenderCurrent writes to outputs/field.json.
type fieldCache struct {
	rt        *fieldapi.Runtime
	dir       string
	freshWait time.Duration

	mu       sync.Mutex
	snap     *fieldSnapshot
	building bool
	done     chan struct{} // closed when the running build ends
	lastErr  error
}

func newFieldCache(rt *fieldapi.Runtime, dir string) *fieldCache {
	c := &fieldCache{rt: rt, dir: dir, freshWait: fieldFreshWait()}
	c.snap = loadDiskSnapshot(dir)
	return c
}

// loadDiskSnapshot reads the field.json left by an earlier build (for
// example before a restart) so the first request after boot can answer at
// once while the fresh build runs. It is always treated as stale.
func loadDiskSnapshot(dir string) *fieldSnapshot {
	body, err := os.ReadFile(filepath.Join(dir, "field.json"))
	if err != nil || len(body) == 0 || !json.Valid(body) {
		return nil
	}
	s := &fieldSnapshot{body: body}
	if rb, err := os.ReadFile(filepath.Join(dir, "projection.current.json")); err == nil {
		var r proj.Receipt
		if json.Unmarshal(rb, &r) == nil {
			s.tip, s.generated = r.TipHash, r.Generated
		}
	}
	if s.generated.IsZero() {
		if fi, err := os.Stat(filepath.Join(dir, "field.json")); err == nil {
			s.generated = fi.ModTime().UTC()
		}
	}
	return s
}

// startLocked starts a background build unless one is running and returns
// the channel closed when the running build ends. c.mu must be held.
func (c *fieldCache) startLocked() chan struct{} {
	if c.building {
		return c.done
	}
	c.building = true
	c.done = make(chan struct{})
	done := c.done
	go c.build(done)
	return done
}

func (c *fieldCache) build(done chan struct{}) {
	stamp := c.rt.LedgerStamp()
	t0 := time.Now()
	receipt, err := renderField(c.rt, c.dir)
	var body []byte
	if err == nil {
		body, err = os.ReadFile(filepath.Join(c.dir, "field.json"))
	}
	took := time.Since(t0)
	c.mu.Lock()
	if err == nil {
		gen := receipt.Generated
		if gen.IsZero() {
			gen = time.Now().UTC()
		}
		c.snap = &fieldSnapshot{body: body, tip: receipt.TipHash, stamp: stamp, generated: gen, took: took}
		c.lastErr = nil
	} else {
		c.lastErr = err
	}
	c.building = false
	close(done)
	c.mu.Unlock()
	if err != nil {
		log.Printf("field.json build failed after %s: %v", took.Round(time.Millisecond), err)
	} else if took > 5*time.Second {
		log.Printf("field.json rebuilt in %s (served from cache between ledger changes)", took.Round(time.Millisecond))
	}
}

// warm starts a build in the background (used at startup).
func (c *fieldCache) warm() {
	c.mu.Lock()
	c.startLocked()
	c.mu.Unlock()
}

// get returns the snapshot to serve and whether it matches the ledger now.
// With no snapshot at all it waits for the build (or the request to end).
func (c *fieldCache) get(ctx context.Context) (*fieldSnapshot, bool, error) {
	stamp := c.rt.LedgerStamp()
	c.mu.Lock()
	if c.snap != nil && c.snap.stamp != "" && c.snap.stamp == stamp {
		s := c.snap
		c.mu.Unlock()
		return s, true, nil
	}
	done := c.startLocked()
	have := c.snap != nil
	c.mu.Unlock()

	if have {
		t := time.NewTimer(c.freshWait)
		defer t.Stop()
		select {
		case <-done:
		case <-t.C:
		case <-ctx.Done():
		}
	} else {
		select {
		case <-done:
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snap == nil {
		if c.lastErr != nil {
			return nil, false, c.lastErr
		}
		return nil, false, errors.New("field projection not built yet")
	}
	return c.snap, c.snap.stamp != "" && c.snap.stamp == stamp, nil
}

// setHeaders adds the snapshot headers. X-Field-Tip is unchanged; the rest
// are additive: when the snapshot was built and whether it is behind the
// ledger right now (a newer build is running).
func (s *fieldSnapshot) setHeaders(h http.Header, fresh bool) {
	h.Set("X-Field-Tip", s.tip)
	if !s.generated.IsZero() {
		h.Set("X-Field-Generated", s.generated.UTC().Format(time.RFC3339))
	}
	h.Set("X-Field-Stale", strconv.FormatBool(!fresh))
	if s.took > 0 {
		h.Set("X-Field-Build-Ms", strconv.FormatInt(s.took.Milliseconds(), 10))
	}
}
