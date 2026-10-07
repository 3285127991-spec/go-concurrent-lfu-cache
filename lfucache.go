// Package lfucache provides a goroutine-safe in-memory LFU cache.
package lfucache

import (
	"container/list"
	"errors"
	"sort"
	"sync"
)

var (
	// ErrInvalidCapacity is returned by New when capacity is not positive.
	ErrInvalidCapacity = errors.New("lfucache: capacity must be greater than zero")
	// ErrEmptyKey is returned by Put when the key is empty.
	ErrEmptyKey = errors.New("lfucache: key must not be empty")
	// ErrClosed is returned by Put when the cache has been closed.
	ErrClosed = errors.New("lfucache: cache is closed")
)

// Entry is a snapshot of a single cached item.
type Entry struct {
	Key       string
	Value     string
	Frequency int
}

type node struct {
	key   string
	value string
	freq  int
	elem  *list.Element
}

// Cache is a goroutine-safe LFU cache with O(1) operations.
type Cache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*node
	freqs    map[int]*list.List
	minFreq  int
	closed   bool
}

// New creates a Cache holding at most capacity items.
func New(capacity int) (*Cache, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCapacity
	}
	return &Cache{
		capacity: capacity,
		items:    make(map[string]*node),
		freqs:    make(map[int]*list.List),
		minFreq:  1,
	}, nil
}

// Put inserts or updates key. Updating an existing key counts as an access.
func (c *Cache) Put(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}
	if n, ok := c.items[key]; ok {
		n.value = value
		c.promote(n)
		return nil
	}
	if len(c.items) == c.capacity {
		c.evict()
	}
	n := &node{key: key, value: value, freq: 1}
	c.addToGroup(n)
	c.items[key] = n
	c.minFreq = 1
	return nil
}

// Get returns the value for key and counts as an access.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", false
	}
	n, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.promote(n)
	return n.value, true
}

// Peek returns the value for key without changing frequency or order.
func (c *Cache) Peek(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	n, ok := c.items[key]
	if !ok {
		return "", false
	}
	return n.value, true
}

// Delete removes key and reports whether it existed.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	n, ok := c.items[key]
	if !ok {
		return false
	}
	c.removeFromGroup(n)
	delete(c.items, key)
	return true
}

// Snapshot returns an independent copy of all entries, ordered by
// eviction candidate first: lowest frequency, then least recently used.
func (c *Cache) Snapshot() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	freqs := make([]int, 0, len(c.freqs))
	for f := range c.freqs {
		freqs = append(freqs, f)
	}
	sort.Ints(freqs)
	entries := make([]Entry, 0, len(c.items))
	for _, f := range freqs {
		lst := c.freqs[f]
		for e := lst.Back(); e != nil; e = e.Prev() {
			n := e.Value.(*node)
			entries = append(entries, Entry{Key: n.key, Value: n.value, Frequency: n.freq})
		}
	}
	return entries
}

// Len returns the number of cached items.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Capacity returns the maximum number of cached items.
func (c *Cache) Capacity() int {
	return c.capacity
}

// Close rejects future Put, Get and Delete calls while keeping
// Peek, Snapshot, Len and Capacity available. It is idempotent.
func (c *Cache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
}

func (c *Cache) promote(n *node) {
	oldFreq := n.freq
	lst := c.freqs[oldFreq]
	lst.Remove(n.elem)
	if lst.Len() == 0 {
		delete(c.freqs, oldFreq)
		if c.minFreq == oldFreq {
			c.minFreq = oldFreq + 1
		}
	}
	n.freq = oldFreq + 1
	c.addToGroup(n)
}

func (c *Cache) addToGroup(n *node) {
	lst, ok := c.freqs[n.freq]
	if !ok {
		lst = list.New()
		c.freqs[n.freq] = lst
	}
	n.elem = lst.PushFront(n)
}

func (c *Cache) removeFromGroup(n *node) {
	lst := c.freqs[n.freq]
	lst.Remove(n.elem)
	if lst.Len() == 0 {
		delete(c.freqs, n.freq)
		if n.freq == c.minFreq {
			c.fixMinFreq()
		}
	}
}

func (c *Cache) fixMinFreq() {
	c.minFreq = 1
	for f := range c.freqs {
		if f < c.minFreq {
			c.minFreq = f
		}
	}
}

func (c *Cache) evict() {
	lst := c.freqs[c.minFreq]
	back := lst.Back()
	n := back.Value.(*node)
	lst.Remove(back)
	if lst.Len() == 0 {
		delete(c.freqs, c.minFreq)
	}
	delete(c.items, n.key)
}
