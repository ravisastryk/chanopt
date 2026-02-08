// Package negative — legitimate channel usage, ZERO diagnostics expected.
package negative

import "context"

// Multi-case select: genuine coordination with context cancellation.
func WorkerPool(ctx context.Context, jobs <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for j := range jobs {
			select {
			case out <- j * 2:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Pipeline stage: transforms input channel to output.
func Square(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			out <- v * v
		}
	}()
	return out
}

// Not a generator — doesn't return a channel.
func FireAndForget(ch chan<- int) {
	go func() {
		for i := 0; i < 10; i++ {
			ch <- i
		}
	}()
}
