# Go broadcast (t.Helper branch)
## Changes
- `Listener.Wait` was changed to reproduce a bug
- `t.Helper` is used in `equal` helper function

## Steps to reproduce an undesired behavior

### Test without catching a bug
Run `go test -run=^TestBroadcastSend$ -count=100 -v`. At least on my computer it does not fail.

### Test with catching a bug
Remove `t.Helper` in `equal` helper function
Run again `go test -run=^TestBroadcastSend$ -count=100 -v`. It fails after some time.

## Explanation
It seems that the usage of mutex in `t.Helper` makes some synchronization across multiple goroutines.