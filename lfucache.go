// Package lfucache implements a goroutine-safe LFU (least frequently used)
// cache with a fixed capacity.
package lfucache

import (
	"container/list"
	"errors"
	"sort"
	"sync"
)

var (
	// ErrInvalidCapacity is returned by New when capacity is not positive.
	ErrInvalidCapacity = errors.New("lfucache: capacity must be > 0")
	// ErrEmptyKey is returned by Put when the key is empty.
	ErrEmptyKey = errors.New("lfucache: key must not be empty")
	// ErrClosed is returned by Put when the cache has been closed.
	ErrClosed = errors.New("lfucache: cache is closed")
)

// Item is an independent copy of one cache entry, as returned by Snapshot.
type Item struct {
	Key       string
	Value     string
	Frequency int
}

// entry is the internal node stored in the key index and in exactly one
// frequency group.
type entry struct {
	key     string
	value   string
	freq    int
	element *list.Element // position inside its frequency group
}

// Cache is a fixed-capacity LFU cache safe for concurrent use.
//
// Internally it keeps a key index (items), per-frequency groups ordered by
// recency (freqs, front = most recently used) and the current minimum
// frequency (minFreq), so Get, Put and Delete never scan all elements.
type Cache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*entry
	freqs    map[int]*list.List // frequency -> group of *entry
	minFreq  int
	closed   bool
}

// New creates a cache with the given capacity, which must be > 0.
func New(capacity int) (*Cache, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCapacity
	}
	return &Cache{
		capacity: capacity,
		items:    make(map[string]*entry),
		freqs:    make(map[int]*list.List),
	}, nil
}

// Put inserts or updates a key. A new key starts with frequency 1; updating
// an existing key counts as an access and raises its frequency. When the
// cache is full, the least frequently used entry is evicted first; ties are
// broken by evicting the least recently used one.
func (c *Cache) Put(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}
	if e, ok := c.items[key]; ok {
		e.value = value
		c.promote(e)
		return nil
	}
	if len(c.items) >= c.capacity {
		c.evict()
	}
	e := &entry{key: key, value: value, freq: 1}
	e.element = c.group(1).PushFront(e)
	c.items[key] = e
	c.minFreq = 1
	return nil
}

// Get returns the value for key and counts as an access: the entry's
// frequency increases by 1 and it becomes the most recently used entry of
// its new frequency group.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || key == "" {
		return "", false
	}
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.promote(e)
	return e.value, true
}

// Peek returns the value for key without changing its frequency or recency.
// It keeps working after Close.
func (c *Cache) Peek(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok {
		return "", false
	}
	return e.value, true
}

// Delete removes key and repairs the frequency groups and minFreq.
// It reports whether the key was present.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || key == "" {
		return false
	}
	e, ok := c.items[key]
	if !ok {
		return false
	}
	c.freqs[e.freq].Remove(e.element)
	delete(c.items, key)
	if e.freq == c.minFreq {
		c.fixMinFreq()
	}
	return true
}

// Snapshot returns an independent copy of all entries ordered by eviction
// priority: the first item is the one that would be evicted next (lowest
// frequency, then least recently used). It keeps working after Close.
func (c *Cache) Snapshot() []Item {
	c.mu.RLock()
	defer c.mu.RUnlock()
	freqs := make([]int, 0, len(c.freqs))
	for f, l := range c.freqs {
		if l.Len() > 0 {
			freqs = append(freqs, f)
		}
	}
	sort.Ints(freqs)
	out := make([]Item, 0, len(c.items))
	for _, f := range freqs {
		l := c.freqs[f]
		for el := l.Back(); el != nil; el = el.Prev() {
			e := el.Value.(*entry)
			out = append(out, Item{Key: e.key, Value: e.value, Frequency: e.freq})
		}
	}
	return out
}

// Len reports the current number of entries.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Capacity reports the maximum number of entries.
func (c *Cache) Capacity() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.capacity
}

// Close rejects further Put, Get and Delete calls. Peek, Snapshot, Len and
// Capacity remain usable. Close is idempotent.
func (c *Cache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

// promote moves e from its current frequency group to the front of the
// next one, maintaining minFreq. Caller must hold the write lock.
func (c *Cache) promote(e *entry) {
	old := e.freq
	c.freqs[old].Remove(e.element)
	e.freq = old + 1
	e.element = c.group(e.freq).PushFront(e)
	// e moved exactly one group up, so if the old minimum group is now
	// empty, old+1 is the new minimum.
	if old == c.minFreq && c.freqs[old].Len() == 0 {
		c.minFreq++
	}
}

// evict removes the least recently used entry of the minFreq group.
// Caller must hold the write lock and the cache must be full.
func (c *Cache) evict() {
	l := c.freqs[c.minFreq]
	el := l.Back()
	l.Remove(el)
	delete(c.items, el.Value.(*entry).key)
}

// group returns the list for freq, creating it on demand.
func (c *Cache) group(freq int) *list.List {
	l, ok := c.freqs[freq]
	if !ok {
		l = list.New()
		c.freqs[freq] = l
	}
	return l
}

// fixMinFreq recomputes minFreq after a Delete removed an entry from the
// minFreq group. It scans frequency groups, never individual elements.
func (c *Cache) fixMinFreq() {
	if len(c.items) == 0 {
		c.minFreq = 0
		return
	}
	if l, ok := c.freqs[c.minFreq]; ok && l.Len() > 0 {
		return
	}
	min := 0
	for f, l := range c.freqs {
		if l.Len() > 0 && (min == 0 || f < min) {
			min = f
		}
	}
	c.minFreq = min
}
