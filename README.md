# chanopt

**Static analyzer that detects Go channel patterns replaceable with mutex/atomic — 8× to 127× faster.**

[![Go Reference](https://pkg.go.dev/badge/github.com/ravisastryk/chanopt.svg)](https://pkg.go.dev/github.com/ravisastryk/chanopt)
[![Go Report Card](https://goreportcard.com/badge/github.com/ravisastryk/chanopt)](https://goreportcard.com/report/github.com/ravisastryk/chanopt)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

chanopt finds goroutines that exist only to produce values into a channel — where the channel's synchronization guarantees are stronger than the computation requires — and recommends the minimal primitive.

Built on [go/analysis](https://pkg.go.dev/golang.org/x/tools/go/analysis). Works with `go vet`, `golangci-lint`, and `gopls` out of the box.

## The Problem

```go
// This costs ~305 ns/op
// hchan lock → sudog alloc → gopark → context switch → goready → unlock
// Plus: 2–4 KB goroutine stack retained forever (Go #19702)
func NewIDGenerator() <-chan int64 {
    ch := make(chan int64)
    go func() { var id int64; for { id++; ch <- id } }()
    return ch
}
```

```go
// This costs ~8 ns/op — one atomic CPU instruction, no goroutine
var counter int64
func NextID() int64 { return atomic.AddInt64(&counter, 1) }
```

**38× faster. chanopt finds the first and recommends the second.**

The channel here doesn't coordinate anything — it's an expensive pipe for a simple counter. This pattern is everywhere: ID generators, round-robins, iterators, config stores, circuit breakers. chanopt catches all 10 variants.

## Quick Start

```bash
go install github.com/ravisastryk/chanopt/cmd/chanopt@latest
go vet -vettool=$(which chanopt) ./...
```

Sample output:

```
server.go:42:2: chanopt: IDGenerator pattern — replace channel with atomic.AddInt64 (~38x speedup, 95% confidence)
lb.go:18:2:    chanopt: RoundRobin pattern — replace channel with sync.Mutex + index (~10x speedup, 90% confidence)
iter.go:7:2:   chanopt: BoundedIterator pattern — replace channel with range-over-func (Go 1.23+) or Next() iterator (~40x speedup, 92% confidence)
```

## Detected Patterns

| Pattern | What It Detects | Replace With | Speedup |
|---------|----------------|-------------|---------|
| **ID Generator** | `i++` in `for { ch <- i }` | `atomic.AddInt64` | ~38× |
| **Round-Robin** | `i = (i+1) % len(s)` cycling through slice | `sync.Mutex` + index | ~10× |
| **Rate Limiter** | `time.Ticker` refilling buffered channel | `sync.Mutex` + token bucket | ~8× |
| **Config Store** | Buffered `chan(1)` drain-and-refill for latest value | `atomic.Pointer` / `atomic.Value` | ~80× |
| **Bounded Iterator** | `for _, v := range slice { ch <- v }; close(ch)` | `range-over-func` or `Next()` | ~40× |
| **Circuit Breaker** | Buffered `chan(1)` holding state enum | `atomic.Int32` | ~127× |
| **Channel Semaphore** | `make(chan struct{}, N)` for concurrency limiting | `x/sync/semaphore.Weighted` | ~8× |
| **Singleton** | Goroutine serving same computed value forever | `sync.Once` | ~19× |
| **Fixed Fan-In** | Merging 2–3 fixed goroutines into one channel | `sync.WaitGroup` + slice | ~8× |
| **Ticker Wrapper** | `for { time.Sleep(d); ch <- struct{}{} }` | `time.NewTicker` directly | ~15× |

## How It Works

Three-stage pipeline, one AST walk per file:

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│   Detect    │────▶│   Classify   │────▶│   Report    │
│ detector.go │     │classifier.go │     │ analyzer.go │
└─────────────┘     └──────────────┘     └─────────────┘
```

1. **Detect** — Find the generator idiom: `make(chan T)` + `go func() { ch <- }` + `return ch`
2. **Classify** — Extract structural indicators (increment, modulo, range, close, time calls) and match against 10 patterns
3. **Report** — Emit diagnostic with pattern name, specific replacement, measured speedup, and confidence

A channel is flagged only when all safety criteria hold:
- Single producer goroutine
- No I/O (net, os, io, database/sql)
- No multi-case select (no context coordination)
- Not a pipeline stage (doesn't range over input channels)
- Function returns the channel (generator idiom)
- Body matches a known pattern with ≥50% confidence

Design priority: Zero false positives > catching every true positive.

## Benchmarks

```bash
cd demos && go test -bench=. -benchmem -count=5
```

| Benchmark | Channel | Optimized | Ratio |
|-----------|---------|-----------|-------|
| IDGen | 305 ns/op | 8 ns/op | 38× |
| RoundRobin | 280 ns/op | 25 ns/op | 11× |
| Config | 160 ns/op | 2 ns/op | 80× |
| Iterator/100 | 15 µs/op | 50 ns/op | 300× |
| CircuitBreaker | 160 ns/op | 1.2 ns/op | 127× |
| Singleton | 160 ns/op | 1.5 ns/op | 107× |

All benchmarks are in `demos/bench_test.go` with both the anti-pattern and optimized implementation side by side.

## Integration

### go vet

```bash
go vet -vettool=$(which chanopt) ./...
```

### golangci-lint

Add to `.golangci.yml`:

```yaml
linters-settings:
  custom:
    chanopt:
      path: chanopt
      description: Detect optimizable channel patterns
      original-url: github.com/ravisastryk/chanopt
```

### gopls / VS Code

Findings appear as inline warnings automatically when chanopt is installed as a go vet tool.

## Architecture

### Why Channels Are Expensive

Every `ch <- v` in Go executes:

```
runtime.chansend1()
  → lock(&c.lock)            // acquire hchan internal mutex
  → if no waiting receiver:
      acquireSudog()          // heap-alloc 96-byte waiter struct
      gopark()                // suspend goroutine → scheduler
  → [receiver arrives]
  → goready(gp)              // wake goroutine, re-enqueue on P
  → unlock(&c.lock)
```

Per-operation cost: ~300–600 ns uncontended.
Per-goroutine cost: 2–4 KB stack, retained until exit (Go #19702).

For a 50K req/s ID generator:
- 305 ns × 50K = 15.25 ms/s CPU wasted
- With atomic: 8 ns × 50K = 0.4 ms/s
- 38× faster with no goroutine or stack overhead

### Stage 1: Detection

Scans top-level function declarations for the generator idiom:

```go
func F() <-chan T {         // returns channel
    ch := make(chan T)      // creates channel locally
    go func() {             // exactly one goroutine
        ch <- value         // sends to channel
    }()
    return ch               // returns same channel
}
```

All five conditions must hold.

### Stage 2: Classification

Single AST walk extracts structural indicators:

| Indicator | AST Signal | Pattern |
|-----------|-----------|---------|
| `hasIncrement` | `i++` | IDGenerator |
| `hasModulo` | `i % N` | RoundRobin |
| `hasIndexExpr` | `slice[i]` | RoundRobin |
| `hasRange` | `for _, v := range` | BoundedIterator |
| `hasClose` | `close(ch)` | BoundedIterator |
| `hasTimeSleep` | `time.Sleep()` | ChanTicker |
| `hasTimeTicker` | `time.NewTicker()` | RateLimiter |
| `infiniteLoop` | `for { }` no cond | IDGen, Ticker |

Safety gates checked before classification:
- `containsMultiCaseSelect` → select ≥2 cases → skip (real coordination)
- `containsIO` → net/os/io/database calls → skip (genuine async I/O)
- `rangesOverChannel` → ranges over input channel → skip (pipeline stage)

### Stage 3: Reporting

Looks up pattern in Registry, emits diagnostic with pattern name, replacement, speedup, and confidence.

## Go Channel Internals

### The hchan Struct

Every `make(chan T)` allocates an `hchan` on the heap (96 bytes on amd64):

```go
type hchan struct {
    qcount   uint           // current items in buffer
    dataqsiz uint           // buffer capacity
    buf      unsafe.Pointer // ring buffer
    elemsize uint16
    closed   uint32
    elemtype *_type
    sendx    uint           // send index
    recvx    uint           // receive index
    recvq    waitq          // blocked receivers (sudog list)
    sendq    waitq          // blocked senders  (sudog list)
    lock     mutex          // protects ALL fields
}
```

### Cost Breakdown

| Operation | Cost | Notes |
|-----------|------|-------|
| Lock acquisition | ~15–25 ns | hchan internal mutex |
| sudog allocation | ~30–50 ns | 96B heap alloc, GC pressure |
| gopark() | ~50–100 ns | Suspend goroutine → scheduler |
| goready() | ~50–100 ns | Wake goroutine, re-enqueue |
| Context switch | ~50–150 ns | Save/restore goroutine stack |
| Memory copy | ~5–20 ns | Element size dependent |
| **Total** | **~200–445 ns** | Uncontended |

### Primitive Comparison

| Primitive | Uncontended | Contended | Memory | Goroutine? |
|-----------|-------------|-----------|--------|------------|
| `ch <- v` (unbuf) | ~300–400 ns | ~500–1000 ns | 2–4 KB + 96 B | Yes |
| `ch <- v` (buf) | ~80–150 ns | ~300–600 ns | 96 B + buf | Yes |
| `sync.Mutex` | ~15–25 ns | ~100–300 ns | 8 B | No |
| `atomic.AddInt64` | ~5–10 ns | ~20–50 ns | 8 B | No |
| `atomic.Pointer.Load` | ~3–5 ns | ~3–5 ns | 8 B | No |
| `sync.Once.Do` (hot) | ~1–2 ns | ~1–2 ns | 12 B | No |

When a channel is used purely for value generation (not coordination), you pay for machinery you never use: lock contention protocol, scheduler involvement, goroutine stack retention, GC pressure from sudog allocations.

## Why chanopt Is Novel

### The Gap in Go Tooling

| Tool | Level | Channel Analysis |
|------|-------|-----------------|
| go vet | Syntax + types | None |
| staticcheck | Types + dataflow | SA1017 only |
| go-critic | AST patterns | Style only |
| semgrep | Structural match | Leak detect only |
| golangci-lint | Aggregator | No channel linters |
| **chanopt** | **Pattern-semantic** | **10 patterns** |

chanopt analyzes the purpose of a channel across goroutine boundaries: whether the goroutine exists solely to produce deterministic values where the channel's synchronization guarantees are stronger than the computation requires.

This is analogous to escape analysis for synchronization overhead — determining if synchronization overhead "escapes" to actual use, then devirtualizing the channel into a specific primitive.

### Real-World Evidence

- Go #48567: Channel iterators 100–500× slower → motivated range-over-func
- etcd #10457: Goroutine leaks from channel coordination patterns
- Kubernetes kubelet: 50+ buffered channels, several fit anti-patterns
- Effective Go: Recommends chan struct{} as semaphore (slower than x/sync/semaphore)

## Contributing

### Adding a New Pattern

1. Add pattern enum and `PatternSpec` to `patterns.go`
2. Add indicator extraction and decision branch to `classifier.go`
3. Add positive test case with `// want` comment in `testdata/src/positive/`
4. Add negative test case in `testdata/src/negative/`
5. Run `go test ./pkg/analyzer/...`

Example:

```go
// patterns.go
const NewPattern Pattern = 11
Registry[NewPattern] = PatternSpec{
    Name:        "NewPattern",
    Replacement: "use sync.Something instead",
    Speedup:     "~20x",
    Confidence:  0.85,
}

// classifier.go
case ind.hasSpecialThing && ind.infiniteLoop:
    return NewPattern, 0.85
```

## Prior Art & References

- [Go #48567](https://go.dev/issue/48567) — Channel-as-iterator benchmarked 100–500× slower than alternatives. Directly motivated `range-over-func` in Go 1.23.
- [Go #19702](https://go.dev/issue/19702) — Blocked goroutines are not garbage collected. Generator goroutines leak permanently if the consumer stops reading.
- [Go #51553](https://go.dev/issue/51553) — Proposal for bulk channel send optimization. chanopt eliminates the channel entirely.
- [Go #52652](https://go.dev/issue/52652) — More efficient channel implementation discussions.
- [Go #32113](https://go.dev/issue/32113) — Reduce P churn in channel operations.
- [etcd #10457](https://github.com/etcd-io/etcd/issues/10457) — Goroutine leaks traced to channel-based coordination that could have been mutexes.

## License

[MIT](LICENSE)
