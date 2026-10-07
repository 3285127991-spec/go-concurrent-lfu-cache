// Command lfudemo demonstrates the lfucache library: frequency
// promotion, same-frequency LRU eviction, Delete, and read-only
// queries after Close.
package main

import (
	"errors"
	"fmt"

	"lfucache"
)

func main() {
	demoPromotion()
	demoSameFreqLRU()
	demoDelete()
	demoClose()
}

func demoPromotion() {
	fmt.Println("== 1. 频率提升: Get 提高频率, 淘汰最低频率 ==")
	c := mustNew(3)
	defer c.Close()
	mustPut(c, "a", "apple")
	mustPut(c, "b", "banana")
	mustPut(c, "c", "cherry")
	mustGet(c, "a")
	mustGet(c, "a")
	mustGet(c, "c")
	fmt.Println("Put a,b,c 后 Get(a)x2, Get(c)x1:")
	printSnapshot(c)
	mustPut(c, "d", "date")
	fmt.Println("Put(d) 触发淘汰, 频率最低的 b 被淘汰:")
	printSnapshot(c)
	fmt.Println()
}

func demoSameFreqLRU() {
	fmt.Println("== 2. 同频率 LRU: 频率相同时淘汰最久未访问 ==")
	c := mustNew(2)
	defer c.Close()
	mustPut(c, "x", "1")
	mustPut(c, "y", "2")
	if v, ok := c.Peek("x"); ok {
		fmt.Printf("Peek(x)=%q 只读取, 不改变频率或顺序\n", v)
	}
	mustPut(c, "z", "3")
	fmt.Println("Put(z) 触发淘汰, x 与 y 频率相同, 最久未访问的 x 被淘汰:")
	printSnapshot(c)
	fmt.Println()
}

func demoDelete() {
	fmt.Println("== 3. Delete: 删除元素并修复频率组 ==")
	c := mustNew(3)
	defer c.Close()
	mustPut(c, "a", "1")
	mustPut(c, "b", "2")
	mustPut(c, "c", "3")
	mustGet(c, "a")
	fmt.Println("初始状态 (a 频率为 2):")
	printSnapshot(c)
	fmt.Printf("Delete(a)=%v, Delete(missing)=%v\n", c.Delete("a"), c.Delete("missing"))
	fmt.Println("删除 a 后:")
	printSnapshot(c)
	fmt.Println()
}

func demoClose() {
	fmt.Println("== 4. Close: 拒绝写操作, 保留只读查询 ==")
	c := mustNew(2)
	mustPut(c, "k", "v")
	c.Close()
	c.Close()
	fmt.Println("Close 已调用 (重复调用安全)")
	if err := c.Put("k2", "v2"); errors.Is(err, lfucache.ErrClosed) {
		fmt.Println("Put 被拒绝:", err)
	}
	if _, ok := c.Get("k"); !ok {
		fmt.Println("Get 被拒绝: 返回未命中")
	}
	if !c.Delete("k") {
		fmt.Println("Delete 被拒绝: 返回 false")
	}
	if v, ok := c.Peek("k"); ok {
		fmt.Printf("Peek 仍可用: k=%q\n", v)
	}
	fmt.Printf("Len=%d Capacity=%d 仍可读\n", c.Len(), c.Capacity())
	printSnapshot(c)
}

func printSnapshot(c *lfucache.Cache) {
	fmt.Printf("  Len=%d/%d, Snapshot (按下一次可能被淘汰的顺序):\n", c.Len(), c.Capacity())
	for i, e := range c.Snapshot() {
		fmt.Printf("    %d. key=%-3s value=%-7s freq=%d\n", i+1, e.Key, e.Value, e.Frequency)
	}
}

func mustNew(capacity int) *lfucache.Cache {
	c, err := lfucache.New(capacity)
	if err != nil {
		panic(err)
	}
	return c
}

func mustPut(c *lfucache.Cache, key, value string) {
	if err := c.Put(key, value); err != nil {
		panic(err)
	}
}

func mustGet(c *lfucache.Cache, key string) {
	if _, ok := c.Get(key); !ok {
		panic("unexpected miss: " + key)
	}
}
