package broadcast_test

import (
	"context"
	"sync"
	"testing"

	. "github.com/nursik/go-broadcast"
)

func equal[T comparable](t *testing.T, expected T, got T) {
	t.Helper()
	if expected != got {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func equalArrays[T comparable, E []T](t *testing.T, expected E, got E) {
	if len(expected) != len(got) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
	for i, v := range expected {
		if got[i] != v {
			t.Fatalf("expected %v, got %v", expected, got)
		}
	}
}

func waitExactlyN(t *testing.T, n int, ch <-chan struct{}) {
	for n > 0 {
		n--
		<-ch
	}

	select {
	case <-ch:
		t.Fatal("should not receive reply")
	default:
	}
}

func TestBroadcastNew(t *testing.T) {
	broadcast := New[int]()

	equal(t, 0, broadcast.Offset())
	equal(t, false, broadcast.IsClosed())
	equal(t, true, broadcast.Send(0))
	equal(t, 1, broadcast.Offset())
	equal(t, true, broadcast.Close())
	equal(t, false, broadcast.Close())
	equal(t, true, broadcast.IsClosed())
	equal(t, false, broadcast.Send(0))
}

func TestBroadcastSend(t *testing.T) {
	broadcast := New[int]()
	// Number of readers and writers
	N := 8
	// Number of messages per writer
	M := 1000

	waitCh := make(chan struct{}, N)
	observedValues := make(chan []int, N)

	for i := 0; i < N; i++ {
		go func() {
			listener := broadcast.NewListener()
			waitCh <- struct{}{}
			values := make([]int, 0, M*N)
			// Total number of messages is M*N. We consume them all and send gathered messages to observedValues channel.
			for j := 0; j < M*N; j++ {
				v, ok := listener.Wait()
				equal(t, true, ok)
				values = append(values, v)
			}
			v, ok := listener.Wait()
			equal(t, false, ok)
			equal(t, 0, v)
			equal(t, true, listener.IsClosed())
			observedValues <- values
		}()
	}
	// Wait for readers
	waitExactlyN(t, N, waitCh)

	for i := 0; i < N; i++ {
		go func() {
			for j := 0; j < M; j++ {
				ok := broadcast.Send(j*N + i)
				equal(t, true, ok)
			}
			waitCh <- struct{}{}
		}()
	}
	// Wait for writers
	waitExactlyN(t, N, waitCh)

	equal(t, N*M, broadcast.Offset())
	broadcast.Close()
	equal(t, false, broadcast.Send(1))

	values1 := <-observedValues

	// Check that all observed values by readers are same and in expected order
	for i := 1; i < N; i++ {
		values2 := <-observedValues
		equalArrays(t, values1, values2)
	}

	previous := make([]int, N)
	for i := 0; i < N; i++ {
		previous[i] = -1
	}
	// Check that order of messages per writer is same
	for _, v := range values1 {
		idx := v / N
		caller := v % N
		equal(t, previous[caller]+1, idx)
		previous[caller] = idx
	}
}

func TestListenerWait(t *testing.T) {
	broadcast := New[int]()
	listener1 := broadcast.NewListener()
	listener2 := broadcast.NewListener()
	waitCh := make(chan struct{}, 1)

	go func() {
		waitCh <- struct{}{}
		v, ok := listener1.Wait()
		equal(t, 1, v)
		equal(t, true, ok)
		waitCh <- struct{}{}
		v, ok = listener1.Wait()
		equal(t, 0, v)
		equal(t, false, ok)
		waitCh <- struct{}{}
	}()
	waitExactlyN(t, 1, waitCh)
	waitExactlyN(t, 0, waitCh)

	broadcast.Send(1)
	waitExactlyN(t, 1, waitCh)

	broadcast.Close()
	waitExactlyN(t, 1, waitCh)

	go func() {
		v, ok := listener2.Wait()
		equal(t, 1, v)
		equal(t, true, ok)
		v, ok = listener2.Wait()
		equal(t, 0, v)
		equal(t, false, ok)
		waitCh <- struct{}{}
	}()
	waitExactlyN(t, 1, waitCh)
}

func TestListenerWaitWithContext(t *testing.T) {
	broadcast := New[int]()
	listener1 := broadcast.NewListener()
	listener2 := broadcast.NewListener()
	waitCh := make(chan struct{}, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		waitCh <- struct{}{}

		v, ok, err := listener1.WaitWithContext(ctx)
		equal(t, 1, v)
		equal(t, true, ok)
		equal(t, nil, err)
		waitCh <- struct{}{}

		v, ok, err = listener1.WaitWithContext(ctx)
		equal(t, 0, v)
		equal(t, false, ok)
		equal(t, context.Canceled, err)
		waitCh <- struct{}{}

		v, ok, err = listener1.WaitWithContext(context.Background())
		equal(t, 0, v)
		equal(t, false, ok)
		equal(t, nil, err)
		waitCh <- struct{}{}
	}()
	waitExactlyN(t, 1, waitCh)
	waitExactlyN(t, 0, waitCh)

	broadcast.Send(1)
	waitExactlyN(t, 1, waitCh)

	cancel()
	waitExactlyN(t, 1, waitCh)

	broadcast.Close()
	waitExactlyN(t, 1, waitCh)

	go func() {
		waitCh <- struct{}{}

		v, ok, err := listener2.WaitWithContext(ctx)
		equal(t, 1, v)
		equal(t, true, ok)
		equal(t, nil, err)
		waitCh <- struct{}{}

		v, ok, err = listener2.WaitWithContext(context.Background())
		equal(t, 0, v)
		equal(t, false, ok)
		equal(t, nil, err)
		waitCh <- struct{}{}
	}()
	waitExactlyN(t, 3, waitCh)
}

func TestBroadcastNewListener(t *testing.T) {
	const N = 10
	broadcast := New[int]()

	listeners := make([]*Listener[int], N)

	for i := 0; i < N; i++ {
		listeners[i] = broadcast.NewListener()
		broadcast.Send(i)
	}

	for i := 0; i < N; i++ {
		listener := listeners[i]
		equal(t, N-i, listener.Len())
		equal(t, i, listener.Offset())
		for j := i; j < N; j++ {
			v, ok := listener.Wait()
			equal(t, j, v)
			equal(t, true, ok)
			equal(t, N-j-1, listener.Len())
			equal(t, j+1, listener.Offset())
		}
		v, ok := listener.Peek()
		equal(t, 0, v)
		equal(t, false, ok)
	}
}

func TestListenerClone(t *testing.T) {
	const N = 10
	broadcast := New[int]()

	listeners := make([]*Listener[int], N)
	listeners[0] = broadcast.NewListener()

	for i := 0; i < N; i++ {
		broadcast.Send(i)
	}
	for i := 1; i < N; i++ {
		prev := listeners[i-1]
		listeners[i] = prev.Clone()
		listeners[i].Peek()
	}

	for i := 0; i < N; i++ {
		listener := listeners[i]
		equal(t, N-i, listener.Len())
		for j := i; j < N; j++ {
			v, ok := listener.Wait()
			equal(t, j, v)
			equal(t, true, ok)
			equal(t, N-j-1, listener.Len())
		}
		v, ok := listener.Peek()
		equal(t, 0, v)
		equal(t, false, ok)
	}
}

func TestListenerWaitHalting(t *testing.T) {
	// This test tests only halting of goroutines
	// Number of readers and writers
	N := 8
	// Number of messages per writer
	M := 10_000

	if testing.Short() {
		M = 100
	}

	broadcast := New[int]()
	waitCh := make(chan struct{}, N)

	for j := 0; j < N; j++ {
		go func() {
			listener := broadcast.NewListener()
			waitCh <- struct{}{}
			for i := 0; i < M; i++ {
				for k := 0; k < N; k++ {
					listener.Wait()
				}
				waitCh <- struct{}{}
			}
			// Blocks until we close broadcast
			listener.Wait()
			waitCh <- struct{}{}
		}()
	}
	// Wait for readers
	waitExactlyN(t, N, waitCh)

	// We send 1 message per writer and then wait for response from readers. Do it M times.
	for j := 0; j < M; j++ {
		// Send N messages - one per writer
		for i := 0; i < N; i++ {
			go func() {
				broadcast.Send(i)
			}()
		}
		// Wait for N replies from readers - one per reader
		waitExactlyN(t, N, waitCh)
	}
	broadcast.Close()
	waitExactlyN(t, N, waitCh)
}

func TestListenerWaitHalting2(t *testing.T) {
	// Number of readers and writers
	N := 8
	// Total runs
	R := 10_000

	if testing.Short() {
		R = 100
	}
	for R > 0 {
		R--
		var wg sync.WaitGroup
		wg.Add(N)

		broadcast := New[int]()

		for j := 0; j < N; j++ {
			listener := broadcast.NewListener()
			go func(lst *Listener[int]) {
				for i := 0; i < N; i++ {
					lst.Wait()
				}
				wg.Done()
			}(listener)
		}

		for i := 0; i < N; i++ {
			go func() {
				broadcast.Send(i)
			}()
		}
		wg.Wait()
	}
}
