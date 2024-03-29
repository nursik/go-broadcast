package broadcast

import (
	"context"
)

// Broadcast struct is responsible for notifying listeners about new messages. All methods are thread-safe and lockless/non-blocking.
type Broadcast[T any] struct {
	messagesList *list[T]
}

// IsClosed reports if a broadcast is closed.
func (b *Broadcast[T]) IsClosed() bool {
	return b.messagesList.closed.Load()
}

// Offset reports how many messages were sent (how many times [Broadcast.Send] was called).
func (b *Broadcast[T]) Offset() int {
	return int(b.messagesList.offset.Load())
}

// Close closes the broadcast. The returned boolean indicates if you are the first one to close the broadcast (first call always returns true, subsequent calls return false).
//
// Example:
//
//	isFirst := broadcast.Close()  // true
//	isSecond := broadcast.Close() // false
func (b *Broadcast[T]) Close() bool {
	return b.messagesList.close()
}

// Send broadcasts a value and notifies listeners (if any) about the new message. The returned boolean indicates if a message was sent or not (only applicable if the broadcast is closed).
//
// This is a non-blocking operation, and concurrent calls do not block each other.
func (b *Broadcast[T]) Send(value T) bool {
	return b.messagesList.send(value)
}

// NewListener returns a new [Listener].
//
//	broadcast.Send(1)
//	broadcast.Send(2)
//	listener := broadcast.NewListener()
//	broadcast.Send(3)
//	v, ok := listener.Wait() // will not be blocked
//	fmt.Println(v, ok) // prints 3 and true
//	v, ok = listener.Wait() // will be blocked until new message arrives
//	// ...other goroutine sends new message (broadcast.Send(4))
//	fmt.Println(v, ok) // prints 4 and true
func (b *Broadcast[T]) NewListener() *Listener[T] {
	l := new(Listener[T])
	l.messagesList = b.messagesList
	l.currentNode = b.messagesList.back.Load()
	l.offset = int(b.messagesList.offset.Load())
	return l
}

// New creates a [Broadcast]
func New[T any]() *Broadcast[T] {
	b := new(Broadcast[T])
	b.messagesList = newList[T]()
	return b
}

// Listener struct is responsible for maintaining its position in the list of messages and receiving or waiting for new ones. An instance of Listener is not safe for concurrent usage; you should either clone it ([Listener.Clone]) or create a new listener ([Broadcast.NewListener]).
type Listener[T any] struct {
	currentNode  *node[T]
	messagesList *list[T]
	offset       int
}

// IsClosed reports if broadcast is closed
func (l *Listener[T]) IsClosed() bool {
	return l.messagesList.closed.Load()
}

// Peek returns value, if there is an unconsumed message, otherwise returns zero value. Never blocks.
//
//	v, ok = listener.Peek() // 0, false
//	broadcast.Send(1)
//	v, ok = listener.Peek() // 1, true
func (l *Listener[T]) Peek() (value T, ok bool) {
	if next := l.currentNode.next.Load(); next != nil {
		value = l.currentNode.value
		l.currentNode = next
		l.offset++
		return value, true
	}
	return value, false
}

// Wait returns unconsumed message (if any) or waits for a new message. Unblocks in case of broadcast closing.
func (l *Listener[T]) Wait() (value T, ok bool) {

	// Check if we have unconsumed message
	if value, ok = l.Peek(); ok {
		return value, ok
	}

	// Note: no more than 2 loops required, but I can't elaborate on that.
	for {
		notify := *l.messagesList.notify.Load()

		// Before starting waiting, check again.
		if value, ok = l.Peek(); ok {
			return value, ok
		}
		select {
		case <-notify:
			if value, ok = l.Peek(); ok {
				return value, ok
			}
		case <-l.messagesList.closeCh:
			return value, ok
		}
	}
}

// WaitWithContext returns unconsumed message (if any) or waits for a new message. Unblocks in case of broadcast closing or context cancellation. If unblocked due to context cancellation, err will be ctx.Err().
func (l *Listener[T]) WaitWithContext(ctx context.Context) (value T, ok bool, err error) {
	if value, ok = l.Peek(); ok {
		return value, ok, nil
	}

	for {
		notify := *l.messagesList.notify.Load()

		if value, ok = l.Peek(); ok {
			return value, ok, nil
		}
		select {
		case <-notify:
			if value, ok = l.Peek(); ok {
				return value, ok, nil
			}
		case <-ctx.Done():
			return value, false, ctx.Err()
		case <-l.messagesList.closeCh:
			value, ok = l.Peek()
			return value, ok, nil
		}
	}
}

// Clone returns a new listener by copying the current one.
//
//	broadcast.Send(1)
//	v, ok = listener1.Wait() // 1, true
//	broadcast.Send(2)
//	listener2 := listener1.Clone()
//	broadcast.Send(3)
//	v, ok = listener1.Wait() // 2, true
//	v, ok = listener2.Wait() // 2, true
//	v, ok = listener1.Wait() // 3, true
//	v, ok = listener2.Wait() // 3, true
func (l *Listener[T]) Clone() *Listener[T] {
	nl := new(Listener[T])
	nl.currentNode = l.currentNode
	nl.messagesList = l.messagesList
	nl.offset = l.offset
	return nl
}

// Len reports number of new (unconsumed) messages. It is an approximate value. This discrepancy can occur if the listener was created during concurrent calls to Broadcast.Send. For more details, read [edge cases]
//
// [edge cases]: https://github.com/nursik/go-broadcast?tab=readme-ov-file#concurrent-new-listener-and-send
func (l *Listener[T]) Len() int {
	d := int(l.messagesList.offset.Load()) - l.offset
	if d > 0 {
		return d
	}
	return 0
}

// Offset reports current position in the list. It is an approximate value. For more details, read [edge cases]
//
// [edge cases]: https://github.com/nursik/go-broadcast?tab=readme-ov-file#concurrent-new-listener-and-send
func (l *Listener[T]) Offset() int {
	return l.offset
}
