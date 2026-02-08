// Package demos benchmarks channel anti-patterns vs optimized replacements.
//
//	go test -bench=. -benchmem -count=5
package demos

import (
	"sync"
	"sync/atomic"
	"testing"
)

// ═══ Pattern 1: ID Generator ═══

func BenchmarkIDGen_Channel(b *testing.B) {
	ch := make(chan int64, 64)
	go func() {
		var id int64
		for { id++; ch <- id }
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ { <-ch }
}

func BenchmarkIDGen_Atomic(b *testing.B) {
	var counter int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ { atomic.AddInt64(&counter, 1) }
}

// ═══ Pattern 2: Round-Robin ═══

func BenchmarkRR_Channel(b *testing.B) {
	items := []string{"a", "b", "c", "d"}
	ch := make(chan string, 64)
	go func() {
		for i := 0; ; i = (i + 1) % len(items) { ch <- items[i] }
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ { <-ch }
}

func BenchmarkRR_Mutex(b *testing.B) {
	items := []string{"a", "b", "c", "d"}
	var mu sync.Mutex
	idx := 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mu.Lock()
		_ = items[idx]
		idx = (idx + 1) % len(items)
		mu.Unlock()
	}
}

// ═══ Pattern 4: Config Store ═══

func BenchmarkConfig_Channel(b *testing.B) {
	ch := make(chan string, 1)
	ch <- "v1"
	b.ResetTimer()
	for i := 0; i < b.N; i++ { v := <-ch; ch <- v }
}

func BenchmarkConfig_AtomicValue(b *testing.B) {
	var store atomic.Value
	store.Store("v1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ { _ = store.Load().(string) }
}

// ═══ Pattern 5: Bounded Iterator ═══

func BenchmarkIter_Channel(b *testing.B) {
	items := make([]int, 100)
	for i := range items { items[i] = i }
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch := make(chan int, 64)
		go func() { defer close(ch); for _, v := range items { ch <- v } }()
		for range ch {}
	}
}

func BenchmarkIter_Direct(b *testing.B) {
	items := make([]int, 100)
	for i := range items { items[i] = i }
	b.ResetTimer()
	for i := 0; i < b.N; i++ { for _, v := range items { _ = v } }
}

// ═══ Pattern 6: Circuit Breaker ═══

func BenchmarkCB_Channel(b *testing.B) {
	ch := make(chan int32, 1)
	ch <- 0
	b.ResetTimer()
	for i := 0; i < b.N; i++ { v := <-ch; ch <- v }
}

func BenchmarkCB_Atomic(b *testing.B) {
	var state atomic.Int32
	b.ResetTimer()
	for i := 0; i < b.N; i++ { state.Load() }
}

// ═══ Pattern 8: Singleton ═══

func BenchmarkSingleton_Channel(b *testing.B) {
	ch := make(chan int, 1)
	ch <- 42
	b.ResetTimer()
	for i := 0; i < b.N; i++ { v := <-ch; ch <- v }
}

func BenchmarkSingleton_Once(b *testing.B) {
	var once sync.Once
	var val int
	b.ResetTimer()
	for i := 0; i < b.N; i++ { once.Do(func() { val = 42 }); _ = val }
}
