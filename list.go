package broadcast

import "sync/atomic"

type node[T any] struct {
	value T
	next  atomic.Pointer[node[T]]
}

type list[T any] struct {
	back    atomic.Pointer[node[T]]
	offset  atomic.Int64
	notify  atomic.Pointer[chan struct{}]
	closeCh chan struct{}
	closed  atomic.Bool
}

func (l *list[T]) send(value T) bool {
	if l.closed.Load() {
		return false
	}
	nd := new(node[T])
	l.offset.Add(1)
	oldBack := l.back.Swap(nd)
	oldBack.value = value
	oldBack.next.Store(nd)

	ch := make(chan struct{})
	oldch := l.notify.Swap(&ch)
	close(*oldch)
	return true
}

func (l *list[T]) close() bool {
	first := !l.closed.Swap(true)
	if first {
		close(l.closeCh)
	}
	return first
}

func newList[T any]() *list[T] {
	l := new(list[T])
	l.closeCh = make(chan struct{})
	l.back.Store(new(node[T]))
	ch := make(chan struct{})
	l.notify.Store(&ch)
	return l
}
