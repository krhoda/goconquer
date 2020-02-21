# goconquer
### Welcome to my growing concurrent toolbox!
Once I've written a pattern too many times, it appears here -- documented, tested, and as generic as Go! will allow. They include:
* A Dynamic version of the built-in Select statement which can listen to `n` channels and have additional channels loaded at runtime.
* An Exponential Backoff Manager, set four options, then call `.Wait()` all you like.
* A fan-in fan-out queue, because I'm never embarassed to be [embarassingly parallel](https://en.wikipedia.org/wiki/Embarrassingly_parallel)

- [DynamicSelect](#DynamicSelect)
  * [What?](#dswhat)
  * [Why?](#dswhy)
  * [How?](#dshow)
- [ExpoBackoffMananger](#ExpoBackoffManager)
  * [What?](#exwhat)

### DynamicSelect
#### What?
<a name="dswhat"/>
`DynamicSelect` (`goconquer/ds`) is a easy-to-test, generic, reliable, abstraction around the golang `select` statement. It solves the following issues:
* Can accept dynamic number of channels to listen to.
* Can load new channels at runtime.
* Can handle the closing channels, including ill-fated attempts to read from a closed channel.
* Always listens to kill commands.
* Can prioritize between channels.
* Can provide visibility into which channels are open.
* Will never close the channels you provide. (Golang suggests the sender close channel, do what thou wilst).

#### Why?
<a name="dswhy"/>
I originally wrote my select statements like this:

``` go
signal := make(chan struct{})
data := make(chan MySpecialStruct)

kill := make(chan os.Signal, 1)
signal.Notify(kill, syscall.SIGINT, syscall.SIGTERM)

go worker(data, signal, kill)

for {
	select {
	case <-kill:
		return
	case <-signal:
	// Do some special computation inline here.
	case x := <-data:
	// Do something with x inline here.
	}
}
```
But then, one day, I had to test my code, so I changed it to:
``` go
signal := make(chan struct{})
data := make(chan MySpecialStruct)

kill := make(chan os.Signal, 1)
signal.Notify(kill, syscall.SIGINT, syscall.SIGTERM)

go worker(data, signal, kill)

for {
	select {
	case <-kill:
		return
	case <-signal:
		f()
	case x := <-data:
		g(x)
	}
}
```
Then I could test `f` and `g` without running a worker. Then one day, I wrote an application where `data` ran every few miliseconds and `kill` would not be read from (very quickly, at least), so then I wrote:
``` go
signal := make(chan struct{})
data := make(chan MySpecialStruct)

kill := make(chan os.Signal, 1)
signal.Notify(kill, syscall.SIGINT, syscall.SIGTERM)

go worker(data, signal, kill)

for {
	select {
	case <-kill:
		return
	default:
		select {
		case <-kill:
			return
		case <-signal:
			f()
		case x := <-data:
			g(x)
		}
	}
}
```
Then, for each iteration of the loop, `kill` was always checked first, so at most a kill command is missed only one iteration. By nesting selects, we can prioritize messages as well. But it's ugly, so them I wrote them this way:
``` go
func cycle(data chan MySpecialStruct, signal chan struct{}, kill chan os.Signal) bool {
	select {
	case <-kill:
		return false
	case <-signal:
		f()
	case x := <-data:
		g(x)
	}

	return true
}
...

signal := make(chan struct{})
data := make(chan MySpecialStruct)

kill := make(chan os.Signal, 1)
signal.Notify(kill, syscall.SIGINT, syscall.SIGTERM)

go worker(data, signal, kill)

for {
	select {
	case <-kill:
		return
	default:
		ok := cycle(data, signal, kill)
		if !ok {
			 return
		}
	}
}
```
Now, everything is testable and broken in to readable bites, and one can even nest cycle calls into smaller and smaller prioritized testable pieces, creating a little functional-flow-chart-state-machine. Then I wrote this set of statements so many times I eventaully wrote this repo.

#### How?
<a name="dshow"/>
DynamicSelect requires the channels (`data` in our example) and handlers (`g`) to use only `interface{}`. This looks like a real interesting case for `contracts` or whatever compiler-level polymorphism that Go introduces later, but for now, to use this general purpose structure (if sending data over the channel) you will need reflection. If reflection is too costly (which isn't often in my experience), consider well-typed nested `cycle`-style functions from above.

To create a DynamicSelect, you will need to provide a `onKillAction` function that takes no arguments (an empty function is fine, but here is the opportunity for on-close clean up) and a slice of `ds.ChannelEntry`structs

``` go
type ChannelEntry struct {
	Channel  chan interface{}
	Handler  HandlerEntry
	OnClose  OnCloseEntry
	IsClosed bool
}
```
`Channel` is predictably the channel to be monitored by the `DynamicSelect`. `IsClosed` is useful because you can extract the status of the channel in a running or stopped `DynamicSelect`, and listening on a closed channel is a runtime panic. `DynamicSelect` uses this to allow you to understand the exact state of it's inner workings, and can contain a closed channel.
`Handler` is a `ds.HandlerEntry`

``` go
type HandlerEntry struct {
	Func func(i interface{})
	Blocking bool
	Priority bool
}
```
The `Func` is the function to be called on the `interface`s emitted from the containing `ChannelEntry.Channel`. All handlers set to `Blocking` will be executed synchronously and no other `Blocking` messages will be read from the queue. However, messages with `Blocking` set to `false` can run parallel to anything. `Priority` determines what order a `Blocking` channel is read from. Again, non-`Blocking` channel can run parallel to anything, including `Priority`!

``` go
type OnCloseEntry struct {
	Func     func()
	Blocking bool
}
```
The `Func` is whatever action you wish to take when the listener to the channel dies. This may not be the same as the `Channel` closing if the provided `Handler` panics or the `DynamicSelect` is told to close. `IsClosed` of the containing `ChannelEntry` tells you the status of the channel. `Blocking` determines whether it is run in `go` routine or not.

Once you have a `[]ChannelEntry`, (good and bad) usage looks like this:

``` go
dysl := ds.NewDynamicSelect(onKill, channels)
isAlive := dysl.IsAlive() // false, the select isn't running
go dysl.Forever()

// Make a new channel to load in
c := ds.ChannelEntry{...}

// DO NOT: load when the select is not running, either put it in initial args or wait after ready.
err := dysl.Load(c)
// err.Error() == "DynamicSelect has not been started, this could otherwise deadlock"

// Wait until it is running to access it.
<-dysl.Ready
chs := dysl.Channels() // chs == channels.

isAlive = dysl.IsAlive() // true, the select is running

err = dysl.Load(c) // Add the channel and handler to the listener pool
// err == nil

chs = dysl.Channels() // Now contains `c`
dysl.Kill() // End all listeners and the select. Do nothing to the contained channels. Does not block. Is thread safe.
isAlive = dysl.IsAlive() // false, kill command was heard.

time.After(time.Second) // As stated, non-blocking, so not all listeners may be stopped yet. But they will ASAP.

// All onKill/onClose actions happen.

// DO NOT: Load the stopped select.
err = dysl.Load(c)
// err.Error() = "DynamicSelect has either halted or is incorrectly initialized."

// but we can still access the last known state of the channels provided:
lastKnownChannelStatus := dysl.Channels()
```

<a name="ExpoBackoffManager"/>

### ExpoBackoffManager

<a name="exwhat"/>

#### What?
Expo(nential)BackoffMananger is a concurrent structure run in it's own `go` routine that allows quick set up of an [exponential back-off strategy](https://en.wikipedia.org/wiki/Exponential_backoff). Configuration is dead simple. It's thread safe, but watch the [thundering herd](https://en.wikipedia.org/wiki/Thundering_herd_problem), if passing to many proccesses and consider composing a wrapper which permits batching.
