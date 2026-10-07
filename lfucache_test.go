package lfucache

import (
	"fmt"
	"sync"
	"testing"
)

func mustNew(t *testing.T, capacity int) *Cache {
	t.Helper()
	c, err := New(capacity)
	if err != nil {
		t.Fatalf("New(%d): %v", capacity, err)
	}
	return c
}

func mustPeek(t *testing.T, c *Cache, key, want string) {
	t.Helper()
	got, ok := c.Peek(key)
	if !ok || got != want {
		t.Fatalf("Peek(%q) = (%q, %v), want (%q, true)", key, got, ok, want)
	}
}

func mustMissing(t *testing.T, c *Cache, key string) {
	t.Helper()
	if v, ok := c.Peek(key); ok {
		t.Fatalf("Peek(%q) = (%q, true), want miss", key, v)
	}
}

func TestValidation(t *testing.T) {
	if _, err := New(0); err != ErrInvalidCapacity {
		t.Fatalf("New(0) err = %v, want ErrInvalidCapacity", err)
	}
	if _, err := New(-3); err != ErrInvalidCapacity {
		t.Fatalf("New(-3) err = %v, want ErrInvalidCapacity", err)
	}
	c := mustNew(t, 1)
	if err := c.Put("", "v"); err != ErrEmptyKey {
		t.Fatalf("Put empty key err = %v, want ErrEmptyKey", err)
	}
	if _, ok := c.Get(""); ok {
		t.Fatal("Get(\"\") should miss")
	}
	if c.Delete("") {
		t.Fatal("Delete(\"\") should be false")
	}
}

func TestLFUEviction(t *testing.T) {
	c := mustNew(t, 2)
	c.Put("a", "1")
	c.Put("b", "2")
	if _, ok := c.Get("a"); !ok { // a: freq 2, b: freq 1
		t.Fatal("Get(a) should hit")
	}
	c.Put("c", "3") // full: evicts b (lowest frequency)
	mustPeek(t, c, "a", "1")
	mustMissing(t, c, "b")
	mustPeek(t, c, "c", "3")
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
}

func TestSameFreqLRUEviction(t *testing.T) {
	c := mustNew(t, 2)
	c.Put("a", "1")
	c.Put("b", "2") // both freq 1, a is least recently used
	c.Put("c", "3") // evicts a
	mustMissing(t, c, "a")
	mustPeek(t, c, "b", "2")
	mustPeek(t, c, "c", "3")
}

func TestPutExistingUpdatesAndPromotes(t *testing.T) {
	c := mustNew(t, 2)
	c.Put("a", "1")
	c.Put("b", "2")
	if err := c.Put("a", "updated"); err != nil { // a: freq 2
		t.Fatalf("Put existing: %v", err)
	}
	c.Put("c", "3") // evicts b, not the just-updated a
	mustPeek(t, c, "a", "updated")
	mustMissing(t, c, "b")
	mustPeek(t, c, "c", "3")
}

func TestPeekDoesNotAffectPolicy(t *testing.T) {
	c := mustNew(t, 2)
	c.Put("a", "1")
	c.Put("b", "2")
	for i := 0; i < 5; i++ { // peeking must not raise frequency or recency
		mustPeek(t, c, "a", "1")
	}
	c.Put("c", "3") // a is still the LRU of freq 1: evicted
	mustMissing(t, c, "a")
	mustPeek(t, c, "b", "2")
	mustPeek(t, c, "c", "3")
}

func TestDeleteRepairsMinFreq(t *testing.T) {
	c := mustNew(t, 2)
	c.Put("a", "1")
	c.Get("a")      // a: freq 2
	c.Put("b", "2") // b: freq 1, minFreq = 1
	if !c.Delete("b") {
		t.Fatal("Delete(b) should be true")
	}
	if c.Delete("b") {
		t.Fatal("Delete(b) again should be false")
	}
	if c.Len() != 1 {
		t.Fatalf("Len = %d, want 1", c.Len())
	}
	// minFreq group 1 is now empty; it must have been repaired to 2.
	c.Put("c", "3") // c: freq 1
	c.Get("c")      // c: freq 2; a and c both freq 2, a is older
	c.Put("d", "4") // evicts a (LRU of freq 2), would break on stale minFreq
	mustMissing(t, c, "a")
	mustPeek(t, c, "c", "3")
	mustPeek(t, c, "d", "4")
}

func TestSnapshot(t *testing.T) {
	c := mustNew(t, 3)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	c.Get("c") // c: freq 2; a,b: freq 1 with a least recently used
	snap := c.Snapshot()
	want := []Item{
		{Key: "a", Value: "1", Frequency: 1},
		{Key: "b", Value: "2", Frequency: 1},
		{Key: "c", Value: "3", Frequency: 2},
	}
	if len(snap) != len(want) {
		t.Fatalf("Snapshot len = %d, want %d", len(snap), len(want))
	}
	for i, item := range want {
		if snap[i] != item {
			t.Fatalf("Snapshot[%d] = %+v, want %+v", i, snap[i], item)
		}
	}
	// The snapshot is an independent copy.
	snap[0].Value = "mutated"
	mustPeek(t, c, "a", "1")
}

func TestClose(t *testing.T) {
	c := mustNew(t, 2)
	c.Put("a", "1")
	c.Close()
	c.Close() // idempotent, must not panic
	if err := c.Put("b", "2"); err != ErrClosed {
		t.Fatalf("Put after Close err = %v, want ErrClosed", err)
	}
	if _, ok := c.Get("a"); ok {
		t.Fatal("Get after Close should miss")
	}
	if c.Delete("a") {
		t.Fatal("Delete after Close should be false")
	}
	// Read-only queries still work.
	mustPeek(t, c, "a", "1")
	if snap := c.Snapshot(); len(snap) != 1 || snap[0].Key != "a" {
		t.Fatalf("Snapshot after Close = %+v", snap)
	}
	if c.Len() != 1 || c.Capacity() != 2 {
		t.Fatalf("Len/Capacity after Close = %d/%d, want 1/2", c.Len(), c.Capacity())
	}
}

func TestConcurrentBasic(t *testing.T) {
	const (
		capacity   = 32
		keySpace   = 64
		goroutines = 8
		ops        = 100
	)
	c := mustNew(t, capacity)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("k%d", (id*ops+i)%keySpace)
				switch i % 5 {
				case 0:
					c.Delete(key)
				case 1, 2:
					c.Get(key)
				default:
					c.Put(key, fmt.Sprintf("v%d", i))
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Len() > capacity {
		t.Fatalf("Len = %d exceeds capacity %d", c.Len(), capacity)
	}
	snap := c.Snapshot()
	if len(snap) != c.Len() {
		t.Fatalf("Snapshot len = %d, Len = %d", len(snap), c.Len())
	}
	seen := make(map[string]bool, len(snap))
	for _, item := range snap {
		if seen[item.Key] {
			t.Fatalf("duplicate key %q in snapshot", item.Key)
		}
		seen[item.Key] = true
		if item.Frequency < 1 {
			t.Fatalf("key %q has invalid frequency %d", item.Key, item.Frequency)
		}
		if v, ok := c.Peek(item.Key); !ok || v != item.Value {
			t.Fatalf("Peek(%q) = (%q, %v), snapshot says %q", item.Key, v, ok, item.Value)
		}
	}
}
