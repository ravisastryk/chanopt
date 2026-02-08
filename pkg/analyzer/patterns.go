// Package analyzer implements the chanopt static analysis tool.
//
// chanopt detects Go channel patterns replaceable with faster
// synchronization primitives (atomic, mutex, sync.Once).
package analyzer

import "fmt"

// Pattern represents a detected channel usage anti-pattern.
type Pattern int

const (
	Unknown Pattern = iota
	IDGenerator
	RoundRobin
	RateLimiter
	ConfigBroadcaster
	BoundedIterator
	CircuitBreaker
	ChanSemaphore
	Singleton
	FixedFanIn
	ChanTicker
)

var patternNames = [...]string{
	"Unknown", "IDGenerator", "RoundRobin", "RateLimiter",
	"ConfigBroadcaster", "BoundedIterator", "CircuitBreaker",
	"ChanSemaphore", "Singleton", "FixedFanIn", "ChanTicker",
}

func (p Pattern) String() string {
	if int(p) < len(patternNames) {
		return patternNames[p]
	}
	return "Unknown"
}

// PatternSpec holds the replacement metadata for a detected pattern.
type PatternSpec struct {
	Replacement string // e.g. "sync/atomic.AddInt64"
	Speedup     string // e.g. "~38x"
	Rationale   string // one-line explanation
}

// Registry is the single source of truth for all pattern metadata.
var Registry = map[Pattern]PatternSpec{
	IDGenerator: {
		"atomic.AddInt64",
		"~38x",
		"counter in infinite loop needs only an atomic increment",
	},
	RoundRobin: {
		"sync.Mutex + index",
		"~10x",
		"modular index cycling needs only a guarded counter",
	},
	RateLimiter: {
		"sync.Mutex + token bucket",
		"~8x",
		"ticker-refilled token slot needs only mutex-guarded math",
	},
	ConfigBroadcaster: {
		"atomic.Pointer / atomic.Value",
		"~80x",
		"latest-value store needs only an atomic pointer swap",
	},
	BoundedIterator: {
		"range-over-func (Go 1.23+) or Next() iterator",
		"~40x",
		"finite iteration needs no goroutine or channel",
	},
	CircuitBreaker: {
		"atomic.Int32",
		"~127x",
		"state enum in buffered chan(1) needs only an atomic int",
	},
	ChanSemaphore: {
		"x/sync/semaphore.Weighted",
		"~8x",
		"concurrency limiting chan struct{} is slower than semaphore",
	},
	Singleton: {
		"sync.Once + value field",
		"~19x",
		"one-time value served via channel needs only sync.Once",
	},
	FixedFanIn: {
		"sync.WaitGroup + append to slice",
		"~8x",
		"merging 2-3 fixed goroutines doesn't need a shared channel",
	},
	ChanTicker: {
		"time.NewTicker directly",
		"~15x",
		"wrapping time.Sleep in goroutine+channel duplicates time.Ticker",
	},
}

func init() {
	// Compile-time guarantee: every non-Unknown pattern has a spec.
	for p := IDGenerator; p <= ChanTicker; p++ {
		if _, ok := Registry[p]; !ok {
			panic(fmt.Sprintf("chanopt: pattern %d (%s) missing from Registry", p, p))
		}
	}
}
