package positive

import "time"

func NewIDGenerator() <-chan int64 {
	ch := make(chan int64) // want `chanopt: IDGenerator pattern`
	go func() {
		var id int64
		for {
			id++
			ch <- id
		}
	}()
	return ch
}

func RoundRobin(backends []string) <-chan string {
	ch := make(chan string) // want `chanopt: RoundRobin pattern`
	go func() {
		for i := 0; ; i = (i + 1) % len(backends) {
			ch <- backends[i]
		}
	}()
	return ch
}

func Iterate(items []int) <-chan int {
	ch := make(chan int) // want `chanopt: BoundedIterator pattern`
	go func() {
		defer close(ch)
		for _, v := range items {
			ch <- v
		}
	}()
	return ch
}

func Heartbeat(d time.Duration) <-chan struct{} {
	ch := make(chan struct{}) // want `chanopt: ChanTicker pattern`
	go func() {
		for {
			time.Sleep(d)
			ch <- struct{}{}
		}
	}()
	return ch
}

func RateLimited(rps int) <-chan struct{} {
	ch := make(chan struct{}, rps) // want `chanopt: RateLimiter pattern`
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(rps))
		defer ticker.Stop()
		for range ticker.C {
			ch <- struct{}{}
		}
	}()
	return ch
}
