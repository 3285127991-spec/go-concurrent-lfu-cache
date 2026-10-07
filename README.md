# go-concurrent-lfu-cache

基于 Go 标准库实现的并发安全 LFU (Least Frequently Used) 缓存，仅包含库、演示 CLI 和自动化测试，无任何第三方依赖。

## 特性

- O(1) 的 Get / Put / Delete：key 索引 + 频率分组双向链表 + minFreq，不扫描全部元素
- 淘汰策略：频率最低者优先；频率相同时淘汰最久未访问 (LRU)
- 新 key 频率从 1 开始；Get 或 Put 已存在 key 视为一次访问，频率 +1
- Peek 只读取，不改变频率或顺序
- 多 goroutine 并发安全 (sync.RWMutex)，通过 go test -race 验证
- Close 后拒绝 Put / Get / Delete，Peek / Snapshot / Len / Capacity 仍可只读查询

## 使用

需要 Go 1.27+。

```go
package main

import (
	"fmt"

	"lfucache"
)

func main() {
	c, err := lfucache.New(2) // capacity 必须 > 0
	if err != nil {
		panic(err)
	}
	defer c.Close()

	c.Put("a", "apple")
	c.Put("b", "banana")
	if v, ok := c.Get("a"); ok { // a 频率提升为 2
		fmt.Println(v)
	}
	c.Put("c", "cherry") // 容量已满, 淘汰频率最低的 b

	for _, e := range c.Snapshot() { // 按下一次可能被淘汰的顺序
		fmt.Println(e.Key, e.Value, e.Frequency)
	}
}
```

## API

| 方法 | 说明 |
| --- | --- |
| New(capacity) (*Cache, error) | 创建缓存，capacity <= 0 返回 ErrInvalidCapacity |
| Put(key, value) error | 插入或更新；空 key 返回 ErrEmptyKey，Close 后返回 ErrClosed |
| Get(key) (string, bool) | 读取并计为一次访问 (频率 +1) |
| Peek(key) (string, bool) | 只读取，不改变频率或顺序 |
| Delete(key) bool | 删除并修复频率组与 minFreq，返回是否存在 |
| Snapshot() []Entry | 按淘汰顺序返回独立副本 (Key / Value / Frequency) |
| Len() int | 当前元素个数 |
| Capacity() int | 容量上限 |
| Close() | 关闭缓存，可重复调用；之后写操作被拒绝，只读查询仍可用 |

## CLI 演示

```sh
go run ./cmd/lfudemo
```

依次演示：频率提升淘汰、同频率 LRU 淘汰、Delete 修复频率组、Close 后只读查询。

## 测试

```sh
go test ./...
go test -race ./...
```

覆盖 LFU 淘汰、同频率 LRU、已有 key 更新提升频率、Peek 不改变策略、Delete/minFreq 修复、Snapshot、Close 与基础并发。

## 设计说明

- items: map[string]*node 作为 key 索引
- freqs: map[int]*list.List 按频率分组，每组双向链表头部为最近使用、尾部为最久未使用
- minFreq: 当前最低频率；提升元素时若其独占 minFreq 组则 minFreq+1，删除元素导致 minFreq 组为空时按现存频率分组重建
- 淘汰：取 freqs[minFreq] 链表尾部元素，O(1) 完成

不包含 TTL、持久化、网络、GUI 或数据库等功能。
