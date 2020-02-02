package ds

import (
	"fmt"
	"log"
	"sync"
)

// DynamicSelect is a concurrency control structure likenable to a dynamic generic select statement with sane defaults.
// Also allows additional case statements to be loaded in during runtime.
// Once running, can accept additional ChannelEntry structs to add to the internal cases.
// The increased flexibility comes with the caveat that it can only deal in interfaces.
// If the cost of serialization is also too high, consider implementing a well-typed version of the select in the Forever/StateMachine functions.
// The tiered structure ensures that first Kill commands are read, then Kill commands and one-time events (in this case, channels closing) are attended to, and finally the all sources of messages are consumed.
// This may seem overly cautious, but a stream reading a message every few milliseconds will noisly ignore numerous kill commands if all channels are in a flat select.
type DynamicSelect struct {
	// Kill is used to signal DynamicSelect to halt.
	// Internal operation ensures that once issued, a kill
	// command will be the next message processed.
	Kill chan interface{}

	// Callback used when Kill is closed/has a message.
	OnKillAction func()

	// A channel used to load additional cases into the DynamicSelect during runtime.
	load chan ChannelEntry

	// A list of channels to manange and how to manage them
	Channels []ChannelEntry
	// Aggregator used to pass through only one message at a time.
	aggregator chan dsWrapper

	// done is used to inform all Listeners kill was heard.
	done chan interface{}

	// Aggregator used to pass through priority messages.
	priorityAggregator chan dsWrapper
	// Aggregator used to pass through close notifications.
	onClose chan dsWrapper
	// alive is used to inform listeners if the main routine has exited.
	alive bool
	// listenerWG is used in clean up to make sure all children process have exited.
	listenerWG sync.WaitGroup
}

// ChannelEntry is utilized to handle writes to and closure of the channel.
// It is assumed the handler accepts the messages written to the channel.
// The OnClose handler is expected to have no arguments.
type ChannelEntry struct {
	Channel chan interface{}
	Handler HandlerEntry
	OnClose OnCloseEntry
}

// HandlerEntry is a function that will be called with the message emitted
// by the associated channel. Blocking determines whether it will be run
// in a goroutine (Blocking = false) or synchronously (Blocking = true), the
// later blocking reading messages from the queue.
// If priority is set to true. will be read during the priority phase.
type HandlerEntry struct {
	Func     func(i interface{})
	Blocking bool
	Priority bool
}

// OnCloseEntry is a function that will be called the associated channel closes.
// Blocking determines whether it will be run in a goroutine (Blocking = false) or
// synchronously (Blocking = true), the latter blocking reading messages from the queue.
// If not blocking, is read during the priority tier.
type OnCloseEntry struct {
	Func     func()
	Blocking bool
}

// Simple way to track channels to handlers.
type dsWrapper struct {
	Index  int
	Target interface{}
}

// NewDynamicSelect uses an action to take on kill command, along with a list of channels to manage and returns a fully initialize DynamicSelect.
func NewDynamicSelect(onKillAction func(), channels []ChannelEntry) *DynamicSelect {
	a := make(chan dsWrapper)
	p := make(chan dsWrapper)
	d := make(chan interface{})
	k := make(chan interface{})
	l := make(chan ChannelEntry)
	o := make(chan dsWrapper)
	return &DynamicSelect{
		Kill:               k,
		OnKillAction:       onKillAction,
		load:               l,
		Channels:           channels,
		aggregator:         a,
		done:               d,
		priorityAggregator: p,
		onClose:            o,
		alive:              true,
	}
}

// Forever runs the DynamicSelect with its current Channels.
// For each channel listed, it will call handlers when messages are received
// and call onClose functions for when they are closed.
// If a message is heard on the DynamicSelect's Kill channel, the select is halted and
// all contained channels are closed.
func (d *DynamicSelect) Forever() {
	// Set up defer for clean up:
	defer d.shutDown()

	// Start funneling messages into aggregator.
	go d.startListeners()

	for {
		// If a kill command is heard in any of the operations...
		d.alive = d.stateMachine()
		if !d.alive {
			// ...bail out!
			return
		}
	}
}

// Load either blocks until the given ChannelEntry is loaded into a running DynamicSelect
// or informs via error that the DynamicSelect has halted.
func (d *DynamicSelect) Load(c ChannelEntry) error {
	if d.alive {
		d.load <- c
		return nil
	}
	return fmt.Errorf("DynamicSelect has either halted or is uninitialized.")
}

// Once all listeners hit done, exit.
func (d *DynamicSelect) shutDown() {
	if r := recover(); r != nil {
		log.Printf("Recovered from panic in main DynamicSelect: %v\n", r)
		log.Println("Attempting normal shutdown.")
	}

	// just making sure.
	d.alive = false
	// alert waiting listeners.
	close(d.done)

	// Tell the outside world we're done.
	d.OnKillAction()

	go d.drainChannels()

	// Wait for internal listeners to halt.
	d.listenerWG.Wait()

	// Make it painfully clear to the GC.
	close(d.aggregator)
	close(d.priorityAggregator)
	close(d.onClose)
	close(d.load)
}

// First, check if a kill command was heard during the previous process...
func (d *DynamicSelect) stateMachine() bool {
	select {
	case <-d.Kill:
		return false

	default:
		return d.priorityMessageState()
	}
}

// Then, check if any channel closed (a one-time event) in addition to priority events and the kill command.
func (d *DynamicSelect) priorityMessageState() bool {
	select {
	case dsw := <-d.onClose:
		d.handleOnClose(dsw)
		return true

	case dsw := <-d.priorityAggregator:
		d.handleInternal(dsw)
		return true

	case <-d.Kill:
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

	case next := <-d.load:
		// Grab the current len, and thus next index.
		nextIndex := len(d.Channels)
		// Add next
		d.Channels = append(d.Channels, next)
		// Create New Listener
		go d.startListener(nextIndex, next)
		return true

	case dsw := <-d.onClose:
		d.handleOnClose(dsw)
		return true

	case <-d.Kill:
		return false
	}
}

func (d *DynamicSelect) startListeners() {
	// For each channel and handler
	for index, entry := range d.Channels {
		// Start a go routine with the current channel
		go d.startListener(index, entry)
	}
}

// Start listener either passes messages to the aggregator channels or calls handlers locally
// Depending on the entry supplied.
func (d *DynamicSelect) startListener(i int, e ChannelEntry) {
	d.listenerWG.Add(1)

	// Clean up on close.
	defer func() {
		// We don't control the channels passed in. We may hit a runtime panic if they are closed.
		if r := recover(); r != nil {
			log.Printf("Recovered but exiting in DynamicSelect select listener. Likely attempted to read on a closed channel, error: %v\n", r)
		}

		// check for Blocking
		if !e.OnClose.Blocking {
			go e.OnClose.Func()
		}

		// Otherwise pass to main handler
		var unit interface{}
		lastMessage := dsWrapper{
			Index:  i,
			Target: unit,
		}
		d.onClose <- lastMessage

		// Free up the waitgroup for shutdown.
		d.listenerWG.Done()
	}()

	for {
		// If using non-blocking handlers, we must check the select
		// we are a proxy of is still alive after the last process.
		if !d.alive {
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
	entry := d.Channels[dsw.Index]

	entry.Handler.Func(dsw.Target)
}

func (d *DynamicSelect) handleOnClose(dsw dsWrapper) {
	// Find the coresponding entry in the array,
	entry := d.Channels[dsw.Index]

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
			_, ok := <-d.onClose
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
}
