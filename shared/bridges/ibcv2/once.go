package ibcv2

import (
	"sync"
	"time"
)

// OnceWithKey is a helper allowing for a function to be called once per set of
// arguments, represented as a 'key'. This is helpful when making an expensive
// call many times concurrently and it is likely to return the same values.
//
// This also provides an errorTimeout, which allows for a timeout on how long
// the error
type OnceWithKey[T any] struct {
	onces          map[any]*onceWithTimeout[T]
	mu             *sync.Mutex
	successTimeout time.Duration
	errorTimeout   time.Duration
}

func NewOnceWithKey[T any](sucessTimeout, errorTimeout time.Duration) *OnceWithKey[T] {
	return &OnceWithKey[T]{
		onces:          make(map[any]*onceWithTimeout[T]),
		mu:             new(sync.Mutex),
		successTimeout: sucessTimeout,
		errorTimeout:   errorTimeout,
	}
}

func (o *OnceWithKey[T]) Do(key any, action func() (T, error)) (T, error) {
	o.mu.Lock()
	once, exists := o.onces[key]
	if !exists || once.IsTimedOut() {
		once = newOnceWithTimeout(action, o.successTimeout, o.errorTimeout)
		o.onces[key] = once
	}
	o.mu.Unlock()

	value, err := once.once()
	if err != nil {
		once.SetError(err)
	}

	o.mu.Lock()
	// dont want to let the map grow forever, so if we see a timed out once
	// here, we remove it from the map (it will be recreated anyways on
	// reinvocation)
	if once.IsTimedOut() {
		delete(o.onces, once)
	}
	o.mu.Unlock()

	return value, err
}

type onceWithTimeout[T any] struct {
	once           func() (T, error)
	errorTimeout   time.Time
	successTimeout time.Time
	err            error
	mu             *sync.RWMutex
}

func newOnceWithTimeout[T any](action func() (T, error), successTimeout, errorTimeout time.Duration) *onceWithTimeout[T] {
	return &onceWithTimeout[T]{
		once:           sync.OnceValues(action),
		errorTimeout:   time.Now().Add(errorTimeout),
		successTimeout: time.Now().Add(successTimeout),
		mu:             new(sync.RWMutex),
	}
}

func (o *onceWithTimeout[T]) IsTimedOut() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	errorTimeout := !time.Now().Before(o.errorTimeout) && o.err != nil
	successTimeout := !time.Now().Before(o.successTimeout) && o.err == nil
	return errorTimeout || successTimeout
}

func (o *onceWithTimeout[T]) SetError(err error) {
	o.mu.Lock()
	o.err = err
	o.mu.Unlock()
}
