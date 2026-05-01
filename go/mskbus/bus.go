package mskbus

import (
	"sync"
)

type Bus interface {
	UseT(handler func(interface{}))
	SubscribeT(handler func(interface{})) uint
	Unsubscribe(token uint)
}

type BusOf[T any] struct {
	mutex       sync.RWMutex
	lastVal     T
	subscribers map[uint]func(T)
	n           uint
	appliers    []func(T, T) (T, bool)
}

func New[T any]() *BusOf[T] {
	return &BusOf[T]{
		subscribers: make(map[uint]func(T)),
	}
}

func (b *BusOf[T]) Subscribe(handler func(T)) uint {
	b.mutex.Lock()
	id := b.n
	b.n++
	b.subscribers[id] = handler
	handler(b.lastVal)
	b.mutex.Unlock()
	return id
}

func (b *BusOf[T]) Unsubscribe(id uint) {
	b.mutex.Lock()
	delete(b.subscribers, id)
	b.mutex.Unlock()
}

// Send sends a value to all bus subscribers. The value must not be modified
// until after the next Send() terminates. See Recycle() for a usage with a
// weaker modification constraint.
func (b *BusOf[T]) Send(x T) {
	b.mutex.Lock()
	b.doSend(x)
	b.mutex.Unlock()
}

// Recycle lets you modify the previous value sent to the bus while blocking
// all callers of `Use()` to avoid race conditions. Nevertheless, it should only
// be used when f() is quite quick: if the modification of the value takes a long
// time, you should use `Send()` with double-buffering instead.
func (b *BusOf[T]) Recycle(f func(old T) T) {
	b.mutex.Lock()
	b.doSend(f(b.lastVal))
	b.mutex.Unlock()
}

// RecycleOr works like Recycle but lets you bail out with an error
func (b *BusOf[T]) RecycleOr(f func(old T) (T, error)) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	val, err := f(b.lastVal)
	if err != nil {
		return err
	}
	b.doSend(val)
	return nil
}

func (b *BusOf[T]) doSend(x T) {
	for _, applier := range b.appliers {
		newVal, keep := applier(b.lastVal, x)
		if !keep {
			return
		}
		x = newVal
	}

	b.lastVal = x

	// TODO 1: instead of using sync.RWMutex, use a more clever implementation
	// that allows atomically downgrading the write-lock to a read-lock, so
	// that Use() can be called concurrently with the following loop that notifies
	// subscribers
	// TODO 2: notify subscribers in parallel. to do this, we could:
	//  - spawn a new goroutine for each subscriber in Subscribe()
	//  - signal these goroutines to notify subscribers from here
	//  - wait for all goroutines to finish notifying subscribers before exiting Send()
	// in practice, we should also optimise the case of one subscriber, where we
	// don't have to do the goroutine signaling thing. so, we could handle all-but-the-first
	// subscribers in goroutintes, while handling just the first subscriber here, as we do now
	for i := range b.subscribers {
		b.subscribers[i](x)
	}
}

func (b *BusOf[T]) Use(f func(T)) {
	b.mutex.RLock()
	f(b.lastVal)
	b.mutex.RUnlock()
}

func (b *BusOf[T]) UseT(f func(interface{})) {
	b.mutex.RLock()
	f(b.lastVal)
	b.mutex.RUnlock()
}

func (b *BusOf[T]) SubscribeT(handler func(interface{})) uint {
	return b.Subscribe(func(x T) { handler(x) })
}

// Apply adds a transormation that will be applied each time a new
// value is sent to the bus. The function will be supplied two arguments:
//   - The previous value that was sent through the bus (after all applied transformations)
//   - The current new value
//
// The transformation should return:
//   - The transformed new value
//   - A boolean whether to keep the value (if false the value will be discarded)
//
// This mechanism can be used to implement simple operations like
// deduplication, moving average, filtering, etc but is not as expressive
// as proper functional programming primitives like map/reduce that allow
// different input/output types
func (b *BusOf[T]) Apply(f func(T, T) (T, bool)) *BusOf[T] {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.appliers = append(b.appliers, f)
	return b
}

// DedupBy makes the bus deduplicate values by comparing them
// with the given test
func (b *BusOf[T]) DedupBy(f func(T, T) bool) *BusOf[T] {
	return b.Apply(func(prev T, x T) (T, bool) {
		return x, !f(prev, x)
	})
}
