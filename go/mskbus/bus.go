package mskbus

import (
	"sync"
)

type Bus interface {
	GetT() interface{}
	SubscribeT(handler func(interface{})) uint
	Unsubscribe(token uint)
}

type BusOf[T any] struct {
	mutex       sync.Mutex
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

func (b *BusOf[T]) Send(x T) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, applier := range b.appliers {
		newVal, keep := applier(b.lastVal, x)
		if !keep {
			return
		}
		x = newVal
	}

	b.lastVal = x
	for i := range b.subscribers {
		b.subscribers[i](x)
	}
}

func (b *BusOf[T]) Get() T {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.lastVal
}

func (b *BusOf[T]) GetT() interface{} {
	return b.Get()
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
