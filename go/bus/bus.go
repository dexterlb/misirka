package bus

import "sync"

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
	b.lastVal = x
	for i := range b.subscribers {
		b.subscribers[i](x)
	}
	b.mutex.Unlock()
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
