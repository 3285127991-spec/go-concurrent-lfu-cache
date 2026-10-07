# go-concurrent-lfu-cache

A goroutine-safe LFU (least frequently used) cache written in pure Go.
Library + small demo CLI + tests. No third-party dependencies, no network,
no TTL, no persistence.

## Layout

- `lfucache.go` — the library (package `lfucache`)
- `cmd/lfudemo/main.go` — demo CLI
- `lfucache_test.go` — tests

## Library usage

```go
c, _ := lfucache.New(2) // capacity must be > 0

c.Put("a", "1")        // new keys start at frequency 1
c.Put("b", "2")
c.Get("a")             // access: a's frequency becomes 2
c.Put("c", "3")        // cache full -> evicts b (lowest frequency)

v, ok := c.Peek("a")   // read only: no frequency/recency change
ok = c.Delete("b")     // removes key, repairs minFreq
items := c.Snapshot()  // []Item{Key, Value, Frequency}, first = next evicted
n := c.Len()
cap := c.Capacity()

c.Close()              // idempotent; Put/Get/Delete now rejected,
                       // Peek/Snapshot/Len/Capacity keep working
```

Rules:

- Keys are non-empty strings; values are strings.
- `Get` and `Put` on an existing key count as an access: frequency +1 and
  the entry becomes the most recently used of its new frequency group.
- Eviction removes the lowest-frequency entry; ties break by least recently
  used.
- Internally: key index + per-frequency recency groups + `minFreq`, so
  `Get`/`Put`/`Delete` never scan all elements.
- Safe for concurrent use from multiple goroutines.

## Run the demo CLI

```sh
go run ./cmd/lfudemo
```

Shows frequency promotion, same-frequency LRU eviction, `Delete`, and
read-only queries after `Close`.

## Run the tests

```sh
go test -race ./...
```

Covers LFU eviction, same-frequency LRU, update-promotes, Peek neutrality,
Delete/minFreq repair, Snapshot, Close semantics, and basic concurrency.
