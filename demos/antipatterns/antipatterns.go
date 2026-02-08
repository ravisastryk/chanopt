// Package antipatterns demonstrates 10 channel anti-patterns chanopt detects.
package antipatterns

import "time"

// 1. IDGenerator — goroutine increments counter, sends to channel.
func NewIDGenerator() <-chan int64 {
	ch := make(chan int64)
	go func() {
		var id int64
		for {
			id++
			ch <- id
		}
	}()
	return ch
}

// 2. RoundRobin — goroutine cycles through backends with modulo.
func RoundRobin(backends []string) <-chan string {
	ch := make(chan string)
	go func() {
		for i := 0; ; i = (i + 1) % len(backends) {
			ch <- backends[i]
		}
	}()
	return ch
}

// 3. RateLimiter — goroutine with ticker refilling token channel.
func RateLimiter(rps int) <-chan struct{} {
	ch := make(chan struct{}, rps)
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rps))
		defer ticker.Stop()
		for range ticker.C {
			select {
			case ch <- struct{}{}:
			default:
			}
		}
	}()
	return ch
}

// 4. ConfigBroadcaster — buffered chan(1) as latest-value store.
func ConfigBroadcaster(initial string) (<-chan string, func(string)) {
	ch := make(chan string, 1)
	ch <- initial
	update := func(v string) {
		select {
		case <-ch:
		default:
		}
		ch <- v
	}
	return ch, update
}

// 5. BoundedIterator — goroutine ranges over slice, closes channel.
func Iterate(items []int) <-chan int {
	ch := make(chan int)
	go func() {
		defer close(ch)
		for _, v := range items {
			ch <- v
		}
	}()
	return ch
}

// 6. CircuitBreaker — buffered chan(1) holding state enum.
type CBChan struct{ ch chan int32 }

func NewCircuitBreaker() *CBChan {
	ch := make(chan int32, 1)
	ch <- 0
	return &CBChan{ch: ch}
}

func (cb *CBChan) State() int32 { s := <-cb.ch; cb.ch <- s; return s }
func (cb *CBChan) Trip()        { <-cb.ch; cb.ch <- 1 }
func (cb *CBChan) Reset()       { <-cb.ch; cb.ch <- 0 }

// 7. ChanSemaphore — buffered channel as concurrency limiter.
func ChanSemaphore(max int) chan struct{} {
	return make(chan struct{}, max)
}

// 8. Singleton — goroutine serves same computed value forever.
func ExpensiveSingleton() <-chan int {
	ch := make(chan int, 1)
	go func() {
		val := 42 * 42
		for {
			ch <- val
		}
	}()
	return ch
}

// 9. FixedFanIn — merge two fixed goroutines into one channel.
func FixedFanIn(a, b <-chan int) <-chan int {
	out := make(chan int)
	go func() { for v := range a { out <- v } }()
	go func() { for v := range b { out <- v } }()
	return out
}

// 10. ChanTicker — wrapping time.Sleep in goroutine + channel.
func Heartbeat(d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for {
			time.Sleep(d)
			ch <- struct{}{}
		}
	}()
	return ch
}
