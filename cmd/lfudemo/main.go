// Command lfudemo demonstrates the lfucache library: frequency promotion,
// same-frequency LRU eviction, Delete, and read-only queries after Close.
package main

import (
	"fmt"

	"lfucache"
)

func printSnapshot(c *lfucache.Cache) {
	fmt.Println("  snapshot (evict order: first = next out):")
	for _, item := range c.Snapshot() {
		fmt.Printf("    key=%-3s value=%-3s freq=%d\n", item.Key, item.Value, item.Frequency)
	}
}

func main() {
	fmt.Println("== 1. Frequency promotion: Get raises frequency ==")
	c, err := lfucache.New(2)
	if err != nil {
		panic(err)
	}
	c.Put("a", "1")
	c.Put("b", "2")
	if v, ok := c.Get("a"); ok {
		fmt.Printf("  Get(a) = %q -> a promoted to freq 2\n", v)
	}
	c.Put("c", "3") // evicts b (freq 1), not a (freq 2)
	fmt.Println("  Put(c) with full cache -> evicts b (lowest frequency)")
	printSnapshot(c)
	c.Close()

	fmt.Println("== 2. Same frequency: least recently used is evicted ==")
	c, _ = lfucache.New(2)
	c.Put("x", "1")
	c.Put("y", "2") // x and y both freq 1, x is older
	c.Put("z", "3") // evicts x
	fmt.Println("  Put(x), Put(y), Put(z) -> evicts x (LRU of freq 1)")
	printSnapshot(c)

	fmt.Println("== 3. Delete removes a key ==")
	if c.Delete("y") {
		fmt.Println("  Delete(y) = true")
	}
	if !c.Delete("y") {
		fmt.Println("  Delete(y) again = false (already gone)")
	}
	fmt.Printf("  Len = %d, Capacity = %d\n", c.Len(), c.Capacity())

	fmt.Println("== 4. Close: writes rejected, reads still work ==")
	c.Close()
	c.Close() // idempotent
	if err := c.Put("w", "9"); err != nil {
		fmt.Printf("  Put after Close -> %v\n", err)
	}
	if _, ok := c.Get("z"); !ok {
		fmt.Println("  Get after Close -> miss")
	}
	if !c.Delete("z") {
		fmt.Println("  Delete after Close -> false")
	}
	if v, ok := c.Peek("z"); ok {
		fmt.Printf("  Peek(z) after Close = %q (read-only queries still work)\n", v)
	}
	fmt.Printf("  Len = %d, Capacity = %d\n", c.Len(), c.Capacity())
	printSnapshot(c)
}
