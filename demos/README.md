# chanopt Demos

```bash
go test -bench=. -benchmem -count=5
```

## Structure

- `antipatterns/` — 10 channel patterns chanopt detects
- `optimized/` — The faster replacement for each pattern
- `bench_test.go` — Side-by-side benchmarks

## Expected Results (approximate, single-core)

| Pattern | Channel | Optimized | Speedup |
|---------|---------|-----------|---------|
| IDGenerator | ~300 ns/op | ~8 ns/op | ~38× |
| RoundRobin | ~280 ns/op | ~25 ns/op | ~10× |
| Config | ~160 ns/op | ~2 ns/op | ~80× |
| Iterator/100 | ~15 µs/op | ~50 ns/op | ~300× |
| CircuitBreaker | ~160 ns/op | ~1.2 ns/op | ~127× |
| Singleton | ~160 ns/op | ~1.5 ns/op | ~19× |
