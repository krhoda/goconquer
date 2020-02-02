package dysl

var unit interface{}

// DynamicSelect is a concurrency control structure likenable to a dynamic generic select statement with sane defaults.
// Also allows additional case statements to be loaded in during runtime.
// Once running, can accept additional ChannelEntry structs to add to the internal cases.
// The increased flexibility comes with the caveat that it can only deal in interfaces.
// If the cost of reflection is too high in the handler functions, consider using structs then serialize in and out of JSON, passing byte-slices through the channels.
// Future API may offer BDynamicSelect.
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
	Load chan ChannelEntry

	// A list of channels to manange and how to manage them
	Channels []ChannelEntry
	// Aggregator used to pass through only one message at a time.
	internal chan dsWrapper
	// Aggregator used to pass through close notifications.
	onClose chan dsWrapper
	// alive is used to inform listeners if the main routine has exited.
	alive bool
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
type HandlerEntry struct {
	Func     func(i interface{})
	Blocking bool
}

// OnCloseEntry is a function that will be called the associated channel closes.
// Blocking determines whether it will be run in a goroutine (Blocking = false) or
// synchronously (Blocking = true), the latter blocking reading messages from the queue.
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
	i := make(chan dsWrapper)
	k := make(chan interface{})
	l := make(chan ChannelEntry)
	o := make(chan dsWrapper)
	return &DynamicSelect{
		Kill:         k,
		OnKillAction: onKillAction,
		Load:         l,
		Channels:     channels,
		internal:     i,
		onClose:      o,
	}
}

// Forever runs the DynamicSelect with its current Channels.
// For each channel listed, it will call handlers when messages are received
// and call onClose functions for when they are closed.
// If a message is heard on the DynamicSelect's Kill channel, the select is halted and
// all contained channels are closed.
func (d *DynamicSelect) Forever() {
	// Start funneling messages into aggregator.
	go d.startListeners()

	for {
		// If a kill command is heard in any of the operations...
		ok := d.stateMachine()
		if !ok {
			// ...bail out!
			return
		}
	}
}

// First, check if a kill command was heard during the previous process...
func (d *DynamicSelect) stateMachine() bool {
	select {
	case <-d.Kill:
		d.OnKillAction()
		return false

	default:
		return d.singleMessageState()
	}
}

// Then, check if any channel closed (a one-time event) in addition to the kill command.
func (d *DynamicSelect) singleMessageState() bool {
	select {
	case dsw := <-d.onClose:
		d.handleOnClose(dsw)
		return true

	case <-d.Kill:
		d.OnKillAction()
		return false

	default:
		return d.allMessageState()
	}
}

// Finally, react to any event FIFO.
func (d *DynamicSelect) allMessageState() bool {
	select {

	case dsw := <-d.internal:
		d.handleInternal(dsw)
		return true

	case next := <-d.Load:
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
		d.OnKillAction()
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
	for {
		// If using non-blocking handlers, we must check the select
		// we are a proxy of is still alive.
		if !d.alive {
			return
		}

		// block to hear the channel.
		x, ok := <-e.Channel

		// break when the channel is closed
		if !ok {
			// check for Blocking
			if !e.OnClose.Blocking {
				e.OnClose.Func()
				return
			}

			// Otherwise pass to main handler
			lastMessage := dsWrapper{
				Index:  i,
				Target: unit,
			}
			d.onClose <- lastMessage
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
		d.internal <- message
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
