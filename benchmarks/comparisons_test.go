package broadcast

import (
	"testing"

	dustin "github.com/dustin/go-broadcast"
	. "github.com/nursik/go-broadcast"
	teivah "github.com/teivah/broadcast"
)

func BenchmarkSend(b *testing.B) {
	// Benchmarks broadcasting/sending b.N messages to N readers
	// Benchmark finishes when all b.N messages are sent and every reader receives b.N messages

	// Number of readers
	const N = 32

	b.Run("impl=dustin", func(b *testing.B) {
		br := dustin.NewBroadcaster(N)
		wait := make(chan struct{}, N)
		for i := 0; i < N; i++ {
			go func() {
				ch := make(chan interface{}, 1)
				br.Register(ch)
				wait <- struct{}{}
				for i := 0; i < b.N; i++ {
					<-ch
				}
				wait <- struct{}{}
			}()
		}
		for i := 0; i < N; i++ {
			<-wait
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			br.Submit(i)
		}
		for i := 0; i < N; i++ {
			<-wait
		}
	})

	b.Run("impl=teivah", func(b *testing.B) {
		br := teivah.NewRelay[interface{}]()
		wait := make(chan struct{}, N)
		for i := 0; i < N; i++ {
			go func() {
				l := br.Listener(1)
				ch := l.Ch()
				wait <- struct{}{}
				for i := 0; i < b.N; i++ {
					<-ch
				}
				wait <- struct{}{}
			}()
		}
		for i := 0; i < N; i++ {
			<-wait
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			br.Notify(i)
		}
		for i := 0; i < N; i++ {
			<-wait
		}
	})

	b.Run("impl=self", func(b *testing.B) {
		br := New[interface{}]()

		wait := make(chan struct{}, N)
		for i := 0; i < N; i++ {
			go func() {
				l := br.NewListener()
				wait <- struct{}{}
				for i := 0; i < b.N; i++ {
					_, _ = l.Wait()

				}
				wait <- struct{}{}
			}()
		}
		for i := 0; i < N; i++ {
			<-wait
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			br.Send(i)
		}
		for i := 0; i < N; i++ {
			<-wait
		}
	})
}
