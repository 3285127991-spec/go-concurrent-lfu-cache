package lfucache_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"lfucache"
)

func mustNew(t *testing.T, capacity int) *lfucache.Cache {
	t.Helper()
	c, err := lfucache.New(capacity)
	if err != nil {
		t.Fatalf("New(%d): %v", capacity, err)
	}
	return c
}

func mustPut(t *testing.T, c *lfucache.Cache, key, value string) {
	t.Helper()
	if err := c.Put(key, value); err != nil {
		t.Fatalf("Put(%q, %q): %v", key, value, err)
	}
}

func mustGet(t *testing.T, c *lfucache.Cache, key, want string) {
	t.Helper()
	got, ok := c.Get(key)
	if !ok || got != want {
		t.Fatalf("Get(%q) = %q, %v; want %q, true", key, got, ok, want)
	}
}

func mustMiss(t *testing.T, c *lfucache.Cache, key string) {
	t.Helper()
	if got, ok := c.Get(key); ok {
		t.Fatalf("Get(%q) = %q, true; want miss", key, got)
	}
}

func TestNewRejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1} {
		if _, err := lfucache.New(capacity); !errors.Is(err, lfucache.ErrInvalidCapacity) {
			t.Fatalf("New(%d) err = %v; want ErrInvalidCapacity", capacity, err)
		}
	}
	c := mustNew(t, 2)
	if c.Len() != 0 || c.Capacity() != 2 {
		t.Fatalf("Len=%d Capacity=%d; want 0/2", c.Len(), c.Capacity())
	}
}

func TestLFUEviction(t *testing.T) {
	c := mustNew(t, 2)
	mustPut(t, c, "a", "1")
	mustPut(t, c, "b", "2")
	mustGet(t, c, "a", "1")
	mustPut(t, c, "c", "3")
	mustMiss(t, c, "b")
	mustGet(t, c, "a", "1")
	mustGet(t, c, "c", "3")
	if c.Len() != 2 {
		t.Fatalf("Len=%d; want 2", c.Len())
	}
}

func TestSameFreqLRUEviction(t *testing.T) {
	c := mustNew(t, 2)
	mustPut(t, c, "a", "1")
	mustPut(t, c, "b", "2")
	mustPut(t, c, "c", "3")
	mustMiss(t, c, "a")
	mustGet(t, c, "b", "2")
	mustGet(t, c, "c", "3")
}

func TestPutExistingUpdatesValueAndFrequency(t *testing.T) {
	c := mustNew(t, 2)
	mustPut(t, c, "a", "1")
	mustPut(t, c, "b", "2")
	mustPut(t, c, "a", "10")
	mustPut(t, c, "c", "3")
	mustMiss(t, c, "b")
	mustGet(t, c, "a", "10")
	mustGet(t, c, "c", "3")
}

func TestPeekDoesNotChangeEvictionOrder(t *testing.T) {
	c := mustNew(t, 2)
	mustPut(t, c, "a", "1")
	mustPut(t, c, "b", "2")
	for i := 0; i < 3; i++ {
		if got, ok := c.Peek("a"); !ok || got != "1" {
			t.Fatalf("Peek(a) = %q, %v; want 1, true", got, ok)
		}
	}
	if _, ok := c.Peek("missing"); ok {
		t.Fatal("Peek(missing) = true; want false")
	}
	for _, e := range c.Snapshot() {
		if e.Frequency != 1 {
			t.Fatalf("Snapshot frequency of %q = %d; want 1 after peeks", e.Key, e.Frequency)
		}
	}
	mustPut(t, c, "c", "3")
	mustMiss(t, c, "a")
	mustGet(t, c, "b", "2")
	mustGet(t, c, "c", "3")
}

func TestDeleteRepairsFreqGroups(t *testing.T) {
	c := mustNew(t, 2)
	mustPut(t, c, "a", "1")
	mustPut(t, c, "b", "2")
	mustGet(t, c, "b", "2")
	if !c.Delete("a") {
		t.Fatal("Delete(a) = false; want true")
	}
	if c.Delete("a") {
		t.Fatal("second Delete(a) = true; want false")
	}
	if c.Len() != 1 {
		t.Fatalf("Len=%d; want 1", c.Len())
	}
	mustPut(t, c, "c", "3")
	mustPut(t, c, "d", "4")
	mustMiss(t, c, "c")
	mustGet(t, c, "b", "2")
	mustGet(t, c, "d", "4")

	c2 := mustNew(t, 3)
	mustPut(t, c2, "x", "1")
	mustPut(t, c2, "y", "2")
	mustPut(t, c2, "z", "3")
	mustGet(t, c2, "y", "2")
	if !c2.Delete("y") {
		t.Fatal("Delete(y) = false; want true")
	}
	mustPut(t, c2, "w", "4")
	mustPut(t, c2, "v", "5")
	mustMiss(t, c2, "x")
	mustGet(t, c2, "z", "3")
	mustGet(t, c2, "w", "4")
	mustGet(t, c2, "v", "5")
}

func TestSnapshot(t *testing.T) {
	c := mustNew(t, 3)
	mustPut(t, c, "a", "1")
	mustPut(t, c, "b", "2")
	mustPut(t, c, "c", "3")
	mustGet(t, c, "a", "1")
	mustGet(t, c, "b", "2")

	snap := c.Snapshot()
	want := []lfucache.Entry{
		{Key: "c", Value: "3", Frequency: 1},
		{Key: "a", Value: "1", Frequency: 2},
		{Key: "b", Value: "2", Frequency: 2},
	}
	if len(snap) != len(want) {
		t.Fatalf("Snapshot len = %d; want %d", len(snap), len(want))
	}
	for i, e := range want {
		if snap[i] != e {
			t.Fatalf("Snapshot[%d] = %+v; want %+v", i, snap[i], e)
		}
	}

	snap[0].Value = "mutated"
	if got, ok := c.Peek("c"); !ok || got != "3" {
		t.Fatalf("Peek(c) = %q, %v after mutating snapshot; want 3, true", got, ok)
	}
}

func TestClose(t *testing.T) {
	c := mustNew(t, 2)
	if err := c.Put("", "v"); !errors.Is(err, lfucache.ErrEmptyKey) {
		t.Fatalf("Put empty key err = %v; want ErrEmptyKey", err)
	}
	mustPut(t, c, "k", "v")
	c.Close()
	c.Close()
	if err := c.Put("k2", "v2"); !errors.Is(err, lfucache.ErrClosed) {
		t.Fatalf("Put after Close err = %v; want ErrClosed", err)
	}
	if _, ok := c.Get("k"); ok {
		t.Fatal("Get after Close = true; want false")
	}
	if c.Delete("k") {
		t.Fatal("Delete after Close = true; want false")
	}
	if got, ok := c.Peek("k"); !ok || got != "v" {
		t.Fatalf("Peek after Close = %q, %v; want v, true", got, ok)
	}
	if snap := c.Snapshot(); len(snap) != 1 || snap[0].Key != "k" {
		t.Fatalf("Snapshot after Close = %+v; want single entry k", snap)
	}
	if c.Len() != 1 || c.Capacity() != 2 {
		t.Fatalf("Len=%d Capacity=%d after Close; want 1/2", c.Len(), c.Capacity())
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := mustNew(t, 16)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("key-%d", (id*31+i)%32)
				switch i % 4 {
				case 0:
					if err := c.Put(key, "v"); err != nil {
						t.Errorf("Put(%q): %v", key, err)
					}
				case 1:
					c.Get(key)
				case 2:
					c.Peek(key)
				case 3:
					c.Delete(key)
				}
				if c.Len() > c.Capacity() {
					t.Errorf("Len=%d exceeds Capacity=%d", c.Len(), c.Capacity())
				}
			}
		}(w)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if got := len(c.Snapshot()); got > c.Capacity() {
				t.Errorf("Snapshot len=%d exceeds Capacity=%d", got, c.Capacity())
			}
		}
	}()
	wg.Wait()
	if c.Len() > c.Capacity() {
		t.Fatalf("final Len=%d exceeds Capacity=%d", c.Len(), c.Capacity())
	}
	if got := len(c.Snapshot()); got != c.Len() {
		t.Fatalf("final Snapshot len=%d != Len=%d", got, c.Len())
	}
	c.Close()
	if err := c.Put("k", "v"); !errors.Is(err, lfucache.ErrClosed) {
		t.Fatalf("Put after Close err = %v; want ErrClosed", err)
	}
}
