// Package optimized shows the faster replacement for each anti-pattern.
package optimized

import (
	"sync"
	"sync/atomic"
	"time"
)

// 1. IDGenerator → atomic.AddInt64
type IDGen struct{ counter int64 }

func (g *IDGen) Next() int64 { return atomic.AddInt64(&g.counter, 1) }

// 2. RoundRobin → sync.Mutex + index
type RoundRobin struct {
	mu       sync.Mutex
	backends []string
	idx      int
}

func NewRoundRobin(backends []string) *RoundRobin {
	return &RoundRobin{backends: backends}
}

func (rr *RoundRobin) Next() string {
	rr.mu.Lock()
	b := rr.backends[rr.idx]
	rr.idx = (rr.idx + 1) % len(rr.backends)
	rr.mu.Unlock()
	return b
}

// 3. RateLimiter → sync.Mutex + token bucket
type TokenBucket struct {
	mu       sync.Mutex
	tokens   int
	max      int
	interval time.Duration
	last     time.Time
}

func NewTokenBucket(rps int) *TokenBucket {
	return &TokenBucket{
		tokens: rps, max: rps,
		interval: time.Second / time.Duration(rps),
		last:     time.Now(),
	}
}

func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	now := time.Now()
	tb.tokens += int(now.Sub(tb.last) / tb.interval)
	if tb.tokens > tb.max {
		tb.tokens = tb.max
	}
	tb.last = now
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}
	return false
}

// 4. ConfigBroadcaster → atomic.Pointer
type ConfigStore[T any] struct{ p atomic.Pointer[T] }

func NewConfigStore[T any](initial T) *ConfigStore[T] {
	cs := &ConfigStore[T]{}
	cs.p.Store(&initial)
	return cs
}

func (cs *ConfigStore[T]) Load() T   { return *cs.p.Load() }
func (cs *ConfigStore[T]) Store(v T) { cs.p.Store(&v) }

// 5. BoundedIterator → direct Next() iterator
type SliceIter[T any] struct {
	items []T
	pos   int
}

func NewSliceIter[T any](items []T) *SliceIter[T] {
	return &SliceIter[T]{items: items}
}

func (it *SliceIter[T]) Next() (T, bool) {
	if it.pos >= len(it.items) {
		var zero T
		return zero, false
	}
	v := it.items[it.pos]
	it.pos++
	return v, true
}

// 6. CircuitBreaker → atomic.Int32
type CircuitBreaker struct{ state atomic.Int32 }

func (cb *CircuitBreaker) State() int32 { return cb.state.Load() }
func (cb *CircuitBreaker) Trip()        { cb.state.Store(1) }
func (cb *CircuitBreaker) Reset()       { cb.state.Store(0) }

// 7. Semaphore → mutex + count
type Semaphore struct {
	mu    sync.Mutex
	cond  *sync.Cond
	count int
	max   int
}

func NewSemaphore(max int) *Semaphore {
	s := &Semaphore{max: max}
	s.cond = sync.NewCond(&s.mu)
	return s
}

func (s *Semaphore) Acquire() {
	s.mu.Lock()
	for s.count >= s.max {
		s.cond.Wait()
	}
	s.count++
	s.mu.Unlock()
}

func (s *Semaphore) Release() {
	s.mu.Lock()
	s.count--
	s.cond.Signal()
	s.mu.Unlock()
}

// 8. Singleton → sync.Once
type Singleton struct {
	once sync.Once
	val  int
}

func (s *Singleton) Get() int {
	s.once.Do(func() { s.val = 42 * 42 })
	return s.val
}

// 9. FixedFanIn → sync.WaitGroup + shared slice
func FanInResults(a, b func() int) []int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []int
	wg.Add(2)
	collect := func(f func() int) {
		defer wg.Done()
		v := f()
		mu.Lock()
		results = append(results, v)
		mu.Unlock()
	}
	go collect(a)
	go collect(b)
	wg.Wait()
	return results
}

// 10. Ticker → time.NewTicker directly
func DirectTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}
