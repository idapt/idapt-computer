package commands

import (
	"sync"
	"time"
)

type Deduper struct {
	mu      sync.Mutex
	entries map[string]dedupEntry
	cap     int
	ttl     time.Duration
}

type dedupEntry struct {
	result    Result
	expiresAt time.Time
}

func NewDeduper(capacity int, ttl time.Duration) *Deduper {
	if capacity <= 0 {
		capacity = 10_000
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Deduper{
		entries: make(map[string]dedupEntry, capacity),
		cap:     capacity,
		ttl:     ttl,
	}
}

func (d *Deduper) Lookup(id string) (Result, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.entries[id]
	if !ok {
		return Result{}, false
	}
	if time.Now().After(e.expiresAt) {
		delete(d.entries, id)
		return Result{}, false
	}
	return e.result, true
}

func (d *Deduper) Remember(id string, r Result) {
	if !r.OK {
		return // only successful results are cached for replay
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.entries) >= d.cap {
		now := time.Now()
		for k, v := range d.entries {
			if now.After(v.expiresAt) {
				delete(d.entries, k)
				break
			}
		}
		if len(d.entries) >= d.cap {
			for k := range d.entries {
				delete(d.entries, k)
				break
			}
		}
	}
	d.entries[id] = dedupEntry{result: r, expiresAt: time.Now().Add(d.ttl)}
}

func (d *Deduper) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.entries)
}
