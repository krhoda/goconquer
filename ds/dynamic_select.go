package ds

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// DynamicSelect is a concurrency control structure likenable to a dynamic generic select statement with sane defaults.
// Also allows additional case statements to be loaded in during runtime.
// Once running, can accept additional ChannelEntry structs to add to the internal cases.
// The increased flexibility comes with the caveat that it can only deal in interfaces.
// If the cost of reflection is too high, consider implementing a well-typed version of the select in the Forever/StateMachine functions.
// The tiered structure ensures that first Kill commands are read, then Kill commands and one-time events (in this case, channels closing) are attended to, and finally the all sources of messages are consumed.
// This may seem overly cautious, but a stream reading a message every few milliseconds will noisly ignore numerous kill commands if all channels are in a flat select.
// Note issueing a kill command will not close the channels being listened to.
type DynamicSelect struct {
	// Callback used when Kill is closed/has a message.
	onKillAction func()

	// A list of channels to manange and how to manage them
	channels []ChannelEntry

	// Aggregator used to pass through only one message at a time.
	aggregator chan dsWrapper

	// A channel used to load additional cases into the DynamicSelect during runtime.
	load chan []ChannelEntry

	// Load guard ensures callers to DynamicSelect.Channels() get a snapshot and don't read/write the same thing.
	loadGuard chan interface{}

	// kill is used to signal DynamicSelect to halt.
	// Internal operation ensures that once issued, a kill
	// command will be the next message processed.
	kill chan interface{}

	// Used to ensure kill isn't called multiple times.
	killGuard chan interface{}

	// Prevents multiple kill commands, and alive getting breifly overriden by a race condition.
	killHeard bool

	// done is an internal kill chan;
	done chan interface{}

	// Aggregator used to pass through priority messages.
	priorityAggregator chan dsWrapper

	// Aggregator used to pass through close notifications.
	onClose chan closeWrapper

	// alive is used to inform listeners if the main routine has exited.
	alive bool

	// running is used to accept loads to prevent client deadlocks.
	running bool

	// listenerWG is used in clean up to make sure all children process have exited.
	listenerWG sync.WaitGroup
}

// ChannelEntry is utilized to handle writes to and closure of the channel.
// It is assumed the handler accepts the messages written to the channel.
// The OnClose handler is expected to have no arguments.
type ChannelEntry struct {
	Channel  chan interface{}
	Handler  HandlerEntry
	OnClose  OnCloseEntry
	IsClosed bool
}

// HandlerEntry is a function that will be called with the message emitted
// by the associated channel.

type HandlerEntry struct {
	Func func(i interface{})

	// Blocking determines whether it will be run in a goroutine (Blocking = false)
	// or synchronously (Blocking = true), the latter blocking reading other messages
	// set to Blocking from the queue.
	// A non-Blocking call may occur duing a Blocking call.
	// Two Blocking calls will never be run concurrently.
	Blocking bool

	// If priority is set to true. will be checked for during the priority phase.
	// Non-blocking calls are processed faster than Priority calls. Setting both to
	// true will result in Non-blocking behavior.
	Priority bool
}

// OnCloseEntry is a function that will be called the associated channel closes.
// Blocking determines whether it will be run in a goroutine (Blocking = false) or
// synchronously (Blocking = true), the latter blocking reading other Blocking
// messages from the queue. If not Blocking, is read during the priority tier.
// It will be called during the shut down of DynamicSelect.
type OnCloseEntry struct {
	Func     func()
	Blocking bool
}

// Simple way to track channels to handlers.
type dsWrapper struct {
	Index  int
	Target interface{}
}

type closeWrapper struct {
	Index int
	Entry ChannelEntry
}

// NewDynamicSelect uses an action to take on kill command, along with a list of channels to manage and returns a fully initialize DynamicSelect.
func NewDynamicSelect(onKillAction func(), channels []ChannelEntry) *DynamicSelect {
	// both aggregators, on close notifier, and internal kill chan.
	a := make(chan dsWrapper)
	p := make(chan dsWrapper)
	o := make(chan closeWrapper)
	d := make(chan interface{})

	// guarded channels
	k := make(chan interface{}, 1)
	kg := make(chan interface{}, 1)
	l := make(chan []ChannelEntry)
	lg := make(chan interface{}, 1)

	// prime the guards.
	kg <- unit
	lg <- unit

	return &DynamicSelect{
		onKillAction:       onKillAction,
		load:               l,
		loadGuard:          lg,
		channels:           channels,
		aggregator:         a,
		alive:              true,
		done:               d,
		kill:               k,
		killGuard:          kg,
		killHeard:          false,
		priorityAggregator: p,
		onClose:            o,
	}
}

// Forever runs the DynamicSelect with its current Channels.
// Will close ready once initialized.
// For each channel listed, it will call handlers when messages are received
// and call onClose functions for when they are closed.
// If a message is heard on the DynamicSelect's Kill channel, the select is halted and
// all contained channels are closed.
func (d *DynamicSelect) Forever(ready chan interface{}) {
	// Set up defer for clean up:
	defer d.shutDown()

	d.running = true

	// Start funneling messages into aggregator.
	d.startListeners()
	close(ready)

	for {
		// If a kill command is heard in any of the operations...
		d.alive = d.stateMachine()
		if !d.alive {
			// ...bail out!
			return
		}
	}
}

// IsAlive reports if the DynamicSelect is running.
func (d *DynamicSelect) IsAlive() bool {
	return d.alive && !d.killHeard
}

// Kill issues a non-blocking, safe kill command to the dynamic select.
func (d *DynamicSelect) Kill() {
	if !d.IsAlive() {
		return
	}

	<-d.killGuard
	if d.IsAlive() {
		d.killHeard = true
		d.kill <- unit
	}
	d.killGuard <- unit
}

// Load either blocks until the given ChannelEntry is loaded into a running DynamicSelect
// or informs via error that the DynamicSelect has halted.
func (d *DynamicSelect) Load(c []ChannelEntry) error {
	if !d.IsAlive() {
		return fmt.Errorf("DynamicSelect has either halted or is uninitialized.")
	}

	if !d.running {
		return fmt.Errorf("DynamicSelect has not been started, this could otherwise deadlock.")
	}

	d.load <- c
	return nil
}

// global empty var.
var unit interface{}

// Once all listeners hit done, exit.
func (d *DynamicSelect) shutDown() {
	if r := recover(); r != nil {
		log.Printf("Recovered from panic in main DynamicSelect: %v\n", r)
		log.Println("Attempting normal shutdown.")
	}

	// just making sure.
	d.killHeard = true
	d.alive = false
	d.running = false
	close(d.done)

	// Tell the outside world we're done.
	d.onKillAction()

	// Handle outstanding requests / a flood of closed messages.
	go d.drainChannels()

	// Wait for internal listeners to halt.
	d.listenerWG.Wait()

	// Make it painfully clear to the GC.
	close(d.aggregator)
	close(d.priorityAggregator)
	close(d.onClose)
}

// First, check if a kill command was heard during the previous process...
func (d *DynamicSelect) stateMachine() bool {
	select {
	case <-d.kill:
		return false

	default:
		return d.priorityMessageState()
	}
}

// Then, check if any channel closed (a one-time event) in addition to priority events and the kill command.
func (d *DynamicSelect) priorityMessageState() bool {
	select {
	case ocw := <-d.onClose:
		go d.updateChannels(ocw)
		d.handleOnClose(ocw.Index)
		return true

	case dsw := <-d.priorityAggregator:
		d.handleInternal(dsw)
		return true

	case <-d.kill:
		return false

	default:
		return d.allMessageState()
	}
}

// Finally, react to any event FIFO.
func (d *DynamicSelect) allMessageState() bool {
	select {

	case dsw := <-d.priorityAggregator:
		d.handleInternal(dsw)
		return true

	case dsw := <-d.aggregator:
		d.handleInternal(dsw)
		return true

	case nextList := <-d.load:
		for _, next := range nextList {
			<-d.loadGuard
			// Grab the current len, and thus next index.
			nextIndex := len(d.channels)
			// Add next
			d.channels = append(d.channels, next)
			d.loadGuard <- unit
			// Create New Listener
			d.listenerWG.Add(1)
			go d.startListener(nextIndex, next)
		}

		return true

	case ocw := <-d.onClose:
		go d.updateChannels(ocw)
		d.handleOnClose(ocw.Index)
		return true

	case <-d.kill:
		return false
	}
}

func (d *DynamicSelect) updateChannels(ocw closeWrapper) {
	<-d.loadGuard
	d.channels[ocw.Index] = ocw.Entry
	d.loadGuard <- unit
}

func (d *DynamicSelect) startListeners() {
	// For each channel and handler
	for index, entry := range d.channels {
		// Start a go routine with the current channel
		d.listenerWG.Add(1)
		go d.startListener(index, entry)
		<-d.loadGuard
		d.channels[index].IsClosed = false
		d.loadGuard <- unit
	}
}

func (d *DynamicSelect) Channels() []ChannelEntry {
	<-d.loadGuard
	c := d.channels
	d.loadGuard <- unit
	return c
}

// Start listener either passes messages to the aggregator channels or calls handlers locally
// Depending on the entry supplied.
func (d *DynamicSelect) startListener(i int, e ChannelEntry) {
	e.IsClosed = false

	// Clean up on close.
	defer func() {
		// We don't control the channels passed in. We may hit a runtime panic if they are closed.
		if r := recover(); r != nil {
			log.Printf("Recovered but exiting in DynamicSelect select listener. Likely attempted to read on a closed channel, error: %v\n", r)

			// This is likely true, but a panic in a handler may trip this.
			e.IsClosed = true
		}

		// check for Blocking
		if !e.OnClose.Blocking {
			go e.OnClose.Func()
		}

		// Otherwise pass to main handler
		lastMessage := closeWrapper{
			Index: i,
			Entry: e,
		}
		d.onClose <- lastMessage

		// Free up the waitgroup for shutdown.
		d.listenerWG.Done()
	}()

	for {
		// If using non-blocking handlers, we must check the select
		// we are a proxy of is still alive after the last process.
		if !d.IsAlive() {
			return
		}

		select {
		// While waiting, listen for overarching kill command.
		case <-d.done:
			return
		// block to hear the channel.
		case x, ok := <-e.Channel:

			// break when the channel is closed
			if !ok {
				// by returning here, we do not propegate
				// the 0 value emmited on channel closure.
				e.IsClosed = true
				return
			}

			// check for Blocking. If not handle locally.
			if !e.Handler.Blocking {
				go e.Handler.Func(x)
				continue
			}

			// otherwise, pass through the value to the main listener.
			message := dsWrapper{
				Index:  i,
				Target: x,
			}

			// based on priority
			if e.Handler.Priority {
				d.priorityAggregator <- message
				continue
			}

			d.aggregator <- message
		}
	}
}

func (d *DynamicSelect) handleInternal(dsw dsWrapper) {
	// Find the coresponding entry in the array,
	<-d.loadGuard
	entry := d.channels[dsw.Index]
	d.loadGuard <- unit

	entry.Handler.Func(dsw.Target)
}

func (d *DynamicSelect) handleOnClose(index int) {
	// Find the coresponding entry in the array,
	<-d.loadGuard
	entry := d.channels[index]
	d.loadGuard <- unit

	entry.OnClose.Func()
}

// Looks awful, but drains all channels in the DynamicSelect while waiting for the WG
// to synchronize with the listeners, then close the channels.
func (d *DynamicSelect) drainChannels() {
	go func() {
		for {
			_, ok := <-d.aggregator
			if ok {
				continue
			}
			return
		}
	}()

	go func() {
		for {
			_, ok := <-d.priorityAggregator
			if ok {
				continue
			}
			return
		}
	}()

	go func() {
		for {
			x, ok := <-d.onClose
			if ok {
				d.handleOnClose(x.Index)
				continue
			}
			return
		}
	}()

	// We know that killHeard is set to true so:
	go func() {

		for {
			_, ok := <-d.kill
			if ok {
				continue
			}
			return
		}
	}()

	// At this point, any call to d.Load will return an error, so we can safely
	// Discard these as outstanding requests that will never be filled.
	go func() {
		for {
			_, ok := <-d.load
			if ok {
				continue
			}
			return
		}
	}()

	// Stack any outstanding attempts to call kill or load
	go func() {
		time.Sleep(time.Second)
		// Then close all channels that don't point internally.
		close(d.kill)
		close(d.killGuard)
		close(d.load)
	}()
}
