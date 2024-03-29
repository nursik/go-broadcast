# Go broadcast
[![Go Reference](https://pkg.go.dev/badge/github.com/nursik/go-broadcast.svg)](https://pkg.go.dev/github.com/nursik/go-broadcast)
[![Go Report Card](https://goreportcard.com/badge/github.com/nursik/go-broadcast)](https://goreportcard.com/report/github.com/nursik/go-broadcast)

Go library for non-blocking broadcast.

## Table of contents
- [Go broadcast](#go-broadcast)
  - [Features](#features)
  - [Quickstart](#quickstart)
    - [Send](#send)
    - [Peek](#peek)
    - [Wait](#wait)
    - [WaitWithContext](#waitwithcontext)
    - [New listeners](#new-listeners)
  - [Examples](#examples)
    - [Unlimited buffered chan](#unlimited-buffered-chan)
    - [Kafka-like consumers](#kafka-like-consumers)
  - [Comparisons to standard approach and other libraries](#comparisons-to-standard-approach-and-other-libraries)
    - [Standard approach](#standard-approach)
      - [Slow listener problem](#slow-listener-problem)
      - [Deadlock problem](#deadlock-problem)
    - [This library's issues](#this-librarys-issues)
    - [Benchmark comparisons of other libraries](#benchmark-comparisons-of-other-libraries)
  - [How it works](#how-it-works)
    - [Overview](#overview)
    - [Clone listener](#clone-listener)
    - [Wait methods](#wait-methods)
    - [Memory consumption](#memory-consumption)
    - [Conclusion](#conclusion)
  - [Edge cases](#edge-cases)
    - [Concurrent close and send](#concurrent-close-and-send)
    - [Concurrent new listener and send](#concurrent-new-listener-and-send)
  - [Bug reports and discussion](#bug-reports-and-discussion)
  - [License](#license)
  - [Fun facts](#fun-facts)

## Features
- Broadcast data of any type
- No need to specify capacity or creating channels
- No locks, no blocking methods, no bottlenecks - senders and receivers are [independent](#how-it-works)
- Faster, safer and no deadlocks (compared to [alternatives](#comparisons-to-standard-approach-and-other-libraries))
- Can be used to create [unlimited buffered chan](#unlimited-buffered-chan)
- Can be used to create [Kafka-like consumers](#kafka-like-consumers)

## Quickstart

```bash
go get github.com/nursik/go-broadcast
```
```go
package main

import (
	"fmt"

	"github.com/nursik/go-broadcast"
)

func main() {
	b := broadcast.New[int]()
	listener := b.NewListener()
	done := make(chan struct{})
	go func() {
		for {
			v, ok := listener.Wait()
			if !ok {
				break
			}
			fmt.Printf("Received: %v\n", v)
		}
		done <- struct{}{}
	}()

	b.Send(1)
	b.Send(2)
	b.Send(3)
	b.Close()
	<-done
}
```
Prints
```
Received: 1
Received: 2
Received: 3
```

### Send
```go
    // Send returns a boolean, which is true, if it sent a message (published it).
    // If broadcast is closed, message is discarded and returns false.
    ok := b.Send(1) 
    b.Close()
    ok = b.Send(2) // false
```
### Peek
Consumes a message, if there are any.
```go
    v, ok := listener.Peek() // 0, false as there are no messages
    b.Send(1)
    v, ok = listener.Peek() // 1, true
    b.Send(2)
    b.Close()
    v, ok = listener.Peek() // 2, true 
    v, ok = listener.Peek() // 0, false 

    // Peek analogy
    var value T
    var ok bool
    select {
    case value, ok = <-ch:
    default:
    }
    return value, ok
```

### Wait
Consumes a message, if there are any. Blocks until new message arrives or broadcast is closed.
```go
    v, ok := listener.Wait() // blocked as there are no messages
    // in background b.Send(1)
    v, ok = listener.Wait() // 1, true
    b.Send(2)
    b.Close()
    v, ok = listener.Wait() // 2, true
    // Wait does not block if broadcast is closed. 
    v, ok = listener.Wait() // 0, false 

    // Wait analogy
    value, ok := <-ch:
    return value, ok
```

### WaitWithContext
Consumes a message, if there are any. Blocks until new message arrives, broadcast is closed or context is cancelled.
```go
    ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
    defer cancel()

    // blocked as there are no messages. Will unblock after one second with 0, false, context.DeadlineExceeded
    v, ok, err := listener.WaitWithContext(ctx) 
    b.Send(1)
    b.Close()
    
    // Even so context is cancelled, WaitWithContext always returns a message if it is available.
    v, ok, err = listener.WaitWithContext(ctx) // 1, true, nil
    // Does not block if broadcast is closed.

    v, ok = listener.WaitWithContext(context.Background()) // 0, false, nil
```
### New listeners
`Listener` instance is not thread safe. You need to create a new listener using `Broadcast.NewListener` or `Listener.Clone`. For more [details](#how-it-works)

`Broadcast.NewListener`:
```go
listener1 := b.NewListener()
listener2 := b.NewListener()

go func(){
    for {
        v, ok := listener1.Wait()
        if !ok {
            break
        }
        fmt.Println("listener1", v)
    }
    fmt.Println("listener1 finished")
}()

go func(){
    for {
        v, ok := listener2.Wait()
        if !ok {
            break
        }
        fmt.Println("listener2", v)
    }
    fmt.Println("listener2 finished")
}()

b.Send(1)
b.Send(2)
b.Close()

// Prints in some order. Order per listener is always the same.
// listener1 1
// listener1 2
// listener2 1
// listener2 2
// listener1 finished
// listener2 finished
```

`Broadcast.NewListener`:
```go
listener1 := b.NewListener()
b.Send(1)
b.Send(2)
b.Close()

offset = listener1.Offset() // 0
v, ok = listener1.Wait() // 1, true
offset = listener1.Offset() // 1

// Create listener by cloning listener1
listener2 := listener1.Clone()

offset = listener1.Offset() // 1
v, ok = listener1.Wait() // 2, true
offset = listener1.Offset() // 2

offset = listener1.Offset() // 2
v, ok = listener1.Wait() // 0, false
offset = listener1.Offset() // 2

// listener1 was created after consuming the first message
offset = listener2.Offset() // 1
v, ok = listener2.Wait() // 2, true
offset = listener2.Offset() // 2

offset = listener2.Offset() // 2
v, ok = listener2.Wait() // 0, false
offset = listener2.Offset() // 2
```

## Examples

### Unlimited buffered chan
```go
func NewUnlimitedChan[T any](in <-chan T, out chan<- T) {
	b := broadcast.New[T]()
	listener := b.NewListener()
	go func() {
		for v := range in {
			b.Send(v)
		}
		b.Close()
	}()
	go func() {
		for {
			v, ok := listener.Wait()
			if !ok {
				break
			}
			out <- v
		}
		close(out)
	}()
}

func main() {
	const N = 100
	in := make(chan int)
	out := make(chan int)

	NewUnlimitedChan(in, out)

	for i := 1; i <= N; i++ {
		in <- i
	}
	close(in)

	// Prints 1 to N
	for v := range out {
		fmt.Println(v)
	}
}
```

### Kafka-like consumers
Disclaimer: name of the example is a little bit misleading, but it is a good analogy.

When you use channels, you can draw parallels with RabbitMQ. But you can use this library, to implement an append-only log with offsets like Apache Kafka... Please read [how it works section](#how-it-works) and [edge case](#concurrent-new-listener-and-send).

```go
// Setup append only log

b := broadcast.New[string]()
head := b.NewListener() // points to the head of log (offset:0)

// generate data by sending 201 messages
for i := 0; i <= 200; i++ {
    if i%100 == 0 {
        b.Send("event100")
    } else {
        b.Send("event")
    }
}
```
We can't directly move to specific offset, but we may use `Peek` or `Wait*` methods to move forward
```go
// start from offset:0
cursor := head.Clone()

for i := 0; i <= 200; i++ {
    event, _ := cursor.Peek()
    if event == "event100" {
        fmt.Println("found event100!") // Prints 3 times at offset 0, 100 and 200
    }
}
// now cursor points to tail and the next Peek will return zero value and Wait* methods will be blocked (there are no values to consume).
// We may start again. Let's create batch processing

cursor = head.Clone()
const messagesPerBatch = 10
for i := 0; i <= 200; i++ {
    if i % messagesPerBatch == 0 {
        listener := cursor.Clone()
        go func(){
            for j := 0; j < messagesPerBatch; j++ {
                // process data
                _, _ = listener.Wait()
            }
        }()
    }
    cursor.Peek()
}

// We created 21 goroutines to process 10 messages each. The last goroutine will be blocked until it receives 9 more messages (there are 201 messages in the log).
for i := 0; i < 9; i++ {
    b.Send("remaining")
}
// now it is unblocked and finished processing
// head still points to offset:0. It means 210 messages will not be garbage collected. If you finished processing these messages, move head forward or set new head (keep in mind offset values are not reset)
for i := 0; i < 210; i++ {
    head.Peek()     
}
// or
head = b.NewListener()
// now head points to tail, which has offset:210

```


## Comparisons to standard approach and other libraries
### Standard approach
First, I will describe the standard approach, as two popular broadcast libraries utilize it.

In Go, we can broadcast a message simply by iterating over all receivers' channels and send data to each of them

In order to preserve order of messages in case of concurrent Broadcast calls, we may either use [sync.Mutex](https://github.com/teivah/broadcast/blob/master/broadcast.go) or [channels](https://github.com/dustin/go-broadcast/blob/master/broadcaster.go). For simplicity, I will use mutex and omit some implementation details.

```go

func (b *Broadcast) RegisterListener(ch chan T) {
    b.mutex.Lock()
    defer b.mutex.Unlock()
    //adds channel to the list
}
func (b *Broadcast) UnregisterListener(ch chan T) {
    b.mutex.Lock()
    defer b.mutex.Unlock()
    //removes channel from the list
}

func (b *Broadcast) Send(data T) {
    b.mutex.Lock()
    defer b.mutex.Unlock()
    for _, r := range b.receivers {
        r <- data
    }
}

```

#### Slow listener problem
As you can see, we are iterating over listeners and send a message. However, if one of listeners is slow, it will block `Send`. Other listeners, which did not receive the message, will also be blocked and wait for the slow listener. Subsequently, other `Send` calls will wait as only one `Send` at any time is performed.
1. Slow listeners can block other listeners and senders.
2. Additionally, senders are blocked by other senders.

The whole broadcast flow is blocked, because of slow listener! That's the least of our issues, if we assume that listener will eventually read/receive a message. But what if it does not? How can we guarantee that listener is still listening? You may suggest to use deadlines and drop a message in such case, but it causes more problems: 
1. Listeners may have inconsistent sequences of messages, as some may miss messages.
2. If a slow listener is still slow or even worse not listening, subsequent broadcasting will be blocked, whenever we try send a message to this listener.

#### Deadlock problem
Problem of deadlock is more severe issue. Example:
```go
loop:
for {
    select {
    case message <- in:
        // process message

    case <-done:
        // cancel listening
        break loop
    }
}

// Deadlock section

// Unsubscribe the channel
broadcast.Unregister(in)
```
The process is relatively straightforward: 1) Read messages until a cancellation signal is received. 2) Unregister the listener. However, in "deadlock section", there could be new messages that may fill `in` channel, as we stopped listening. Sender is blocked and broadcast instance too (whole broadcasting instance is guarded by mutex or other synchronization mechanisms). It makes us unable to unregister and enter a deadlock!

One of the solutions is to perform "drain", before trying to unregister:
```go
loop:
for {
    select {
    case message <- in:
        // process message

    case <-done:
        // cancel listening
        break loop
    }
}

go func(){
    for range in {
    }
}

// Unsubscribe the channel
broadcast.Unregister(in)
// close "in" otherwise we will have hanging goroutine
close(in)
```
It is a common solution to this problem, but we may forget to add "drain" (and most tutorials/implementations of broadcast don't mention it).

### This library's issues
You may read more on how [this library works](#how-it-works) and circumvents the above problems. However, this library has potential problems, if not handled correctly:
1) Listeners and senders do not know about existence of each other. It makes the slowest listener fall behind more and more, as time passes. You need some notification mechanism (to notify senders to slow down sending) or create more working goroutines for the slowest listener to process messages faster.
2) Memory leak may occur if you hold a reference to a listener, when you do not need to. Listener will hold references to all *reachable* messages, which may prevent garbage collection.

### Benchmark comparisons of other libraries
We will compare against these two popular libraries:
1) [github.com/dustin/go-broadcast](https://github.com/dustin/go-broadcast) ~240 stars
2) [github.com/teivah/broadcast](https://github.com/teivah/broadcast) ~150 stars

The benchmark is simple: one sender sends N messages; there are `readers` number of listeners waiting for these N messages; benchmark finishes when sender sends all N messages and each listeners receive N messages.
Check [comparisons_test](./benchmarks/comparisons_test.go)

| readers \ library     | self        | dustin                 | teivah                |
| --------------------- | ----------- | ---------------------- | --------------------- |
| 1                     | 252.1n ± 1% | 1221.0n ± 2%  +384.33% | 420.8n ± 1%  +66.92%  |
| 8                     | 1.173µ ± 1% | 3.226µ ± 3%  +175.10%  | 3.188µ ± 3%  +171.86% |
| 32                    | 1.755µ ± 2% | 7.554µ ± 2%  +330.55%  | 7.818µ ± 2%  +345.60% |

## How it works
This is a high-level description and may not necessarily capture the precise details of its low-level implementation or order of operations. 
### Overview
Under the hood, it uses a singly linked list and two channels - `notify` to notify waiting listeners about new messages and `closeCh` to unblock waiting listeners. Both channels are of `chan struct{}` type. Nodes of the linked list contain a `value` of user specified type T and a pointer `next` pointing to the next node.

Broadcast is just a wrapper of this list and initially contains a tail node
```
  broadcast (offset: 0)    
      │        
      ▼        
     tail ─► nil
```
When we send data "value1", we first store data in the tail node, then set the next pointer to a tail. After, creating a new tail, broadcast closes notify channel and creates a new one.

```                
        broadcast (offset:1)      
           │           
           ▼           
  value1─►tail─►nil    
```
"value1" is not reachable by any listener (as there are none) and it will be garbage collected.

```
  broadcast (offset:1)      
     │           
     ▼           
   tail─►nil
```
Create a new listener "listener1"
``` 
 broadcast (offset:1)    
     │         
     ▼         
    tail─►nil  
     ▲         
     │         
 listener1 (offset:1)         
```
New listener will be created with the same offset=1. Offset of a listener may [differ](#concurrent-new-listener-and-send) from broadcast's offset. It does not affect the implementation or correctness of program, as it is used only to inform user about listener's current offset and how many unconsumed messages are left (`Len`).

Send "value1" again.
```
        broadcast (offset:2)
            │               
            ▼               
   value1─►tail─►nil        
     ▲                      
     │                      
 listener1 (offset:1)                           
```
Listener1 is pointing to "value1", which was a tail before sending. Let's consume a message (via `Peek`, `Wait` or `WaitWithContext`)
```
 broadcast (offset:2)   
     │                  
     ▼                  
    tail─►nil           
     ▲                  
     │                  
 listener1 (offset:2)   
```
Consuming message will move listener forward. Any of the above methods used for consuming a message will return "value1", true. 

As there are no messages in the list, calling `Peek` will return zero value and false. `Wait*` methods will block until new message arrives (or broadcast is closed/context is cancelled).

### Clone listener
Previous section shows how a listener created using `Broadcast.NewListener` works. Now, let's examine how `Listener.Clone` works by using the code below:
```go
b := broadcast.New[string]()
listener1 := b.NewListener()
b.Send("value1")
b.Send("value2")
b.Send("value3")
listener2 := listener1.Clone()
```

```
 listener1 (offset:0)   broadcast (offset:3)
     │                      │               
     ▼                      ▼               
   value1─►value2─►value3─►tail─►nil        
     ▲                                      
     │                                      
 listener2 (offset:0)                                             
```
When you clone a listener it will point to the same node and have the same offset.

Consume a message for listener2
```
 listener1 (offset:0)   broadcast (offset:3)
     │                      │               
     ▼                      ▼               
   value1─►value2─►value3─►tail─►nil        
             ▲                              
             │                              
         listener2 (offset:1)               
```
Listener2 moved forward.

Create listener3 by cloning listener2
```
  listener1 (offset:0)   broadcast (offset:3)  
      │                      │                 
      ▼                      ▼                 
    value1─►value2─►value3─►tail─►nil          
              ▲ ▲                              
              │ └───────────────────┐          
              │                     │          
  listener2 (offset:1)    listener3 (offset:1)   
```
Listener3 is exact copy of listener2. If we consume all messages for listener2 and listener3, they will receive "value2" and "value3", and point to tail. Because listener1 still points to "value1", all messages will not be garbage collected, as all nodes are *reachable* by listener1. It makes listener1 some sort of head/front of the list in this situation.

As you can see, listeners are just pointers, cursors, positions in the list. You may use this "feature" to implement [Kafka like consumers](#kafka-like-consumers).

### Wait methods
When you call wait methods on a listener pointing to non-tail node, it will immediately consume a message. If the node is a tail node, it will block until new message arrives or broadcast is closed. It does so by listening `notify` and `closeCh` channels. Because, sending a message closes notify (and creates a new one for future notifications), wait methods are able to receive notifications and unblock to consume new messages.

### Memory consumption
Every time we need to broadcast data, we need to create a node (size of T + 8 bytes for next pointer), create a notify channel and close previous notify channel. With the size of notify channel around ~96 bytes, we allocate around size of T + 104 bytes.

Because we always close previous notify channel, at any point of time we use N * (size of T + 8 bytes) + 96 bytes, where N is the number of unique nodes reachable for listeners. Also we may calculate N as `broadcast.Offset() - min(listener1.Offset(), listener2.Offset(), ...)`.

If there are no listeners, all nodes are not reachable (except the tail, which is stored by broadcast) and they will be garbage collected.

### Conclusion
Using singly linked list (and atomic operations to add nodes) makes all operations independent of each other. Senders and listeners do not know about other senders or listeners. There are no mutexes (except channels, which under the hood use mutexes), no channels to pass data (we use channels only to notify), no slow listeners blocking the whole broadcast flow.

## Edge cases
### Concurrent close and send
Concurrent calls to both `Broadcast.Close` and `Broadcast.Send` are safe. However, it may cause waiting listeners (`Listener`'s `Wait` and `WaitWithContext` methods) to not receive the last messages.

Example:
1. Goroutine 1 waits for a message 
2. Goroutine 2 sends a message
3. Goroutine 3 closes a broadcast
4. Goroutine 1 receives a closing signal (via `<-closeCh`) before receiving a new message signal. It returns zero value and false
5. Goroutine 2 finishes sending a message

Note that:
1. To avoid such situation you need to close broadcast after finishing sending messages.
2. Listeners that are not waiting or still consuming previous messages are not affected; they should observe a value sent by goroutine 2.
 
### Concurrent new listener and send
Concurrent calls to both `Broadcast.NewListener` and `Broadcast.Send` are safe. However, it may result in inaccuracies for `Listener.Len` and `Listener.Offset` for created listeners and their subsequent clones.

- `Broadcast.NewListener` consists of two atomic operations: load tail (back) node and load offset (total number of messages sent).
- `Broadcast.Send` consists of several atomic operations: swap tail (back) node, increment offset (total number of messages sent and others).

As you can see, if above operations are overlapped, it is possible for a new listener to have wrong offset value. Moreover, all new listeners created (`Listener.Clone`) from this listener will report wrong `Offset` and `Len`.

Note that:
1. `Len`'s reported value may differ from true value by N, where N is the number of concurrent sends during creation.
2. `Len` is computed as `Broadcast.Offset` - `Listener.Offset`.
2. `Len` never reports negative values.

## Bug reports and discussion
<img src="https://raw.githubusercontent.com/MariaLetta/free-gophers-pack/master/characters/svg/49.svg" width="150" height="150">

If you encountered unexpected/non-documented behavior, bugs or have questions, feel free to create an issue.

## License
This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Fun facts
- Spent 95% of time writing documentation and README. Number of lines of code is around 160... I hope there is not a single bug
- You should not use `t.Helper` (t *testing.T) in helper functions, if you are testing concurrent programs. Check `thelper` branch for more details
- You can send values bigger than 2^16 bytes unlike channels.