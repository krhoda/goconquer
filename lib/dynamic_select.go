package goconquer

type DynamicSelect struct {
	Kill         chan interface{}
	OnKillAction func()
	Channels     []ChannelEntry
	internal     chan dsWrapper
}

type ChannelEntry struct {
	Channel chan interface{}
	Handler func(i interface{})
}

type dsWrapper struct {
	Index  int
	Target interface{}
}

func (d *DynamicSelect) Forever() {
	d.internal = make(chan interface{}, 1)
	go d.startListeners()
	for {
		select {
		case <-d.Kill:
			d.OnKillAction()
			return
		default:
			select {
			case <-d.Kill:
				d.OnKillAction()
				return
			case dsw := <-d.internal:
				d.handleInternal(dsw)
			}
		}
	}
}

func (d *DynamicSelect) startListeners() {
	for index, entry := range d.Channels {
		go func(i int, e ChannelEntry) {
			for x := range e.Channel {
				message := dsWrapper{
					Index:  i,
					Target: x,
				}
				d.internal <- message
			}
		}(index, entry)
	}
}

func (d *DynamicSelect) handleInternal(dsw dsWrapper) {
	// Find the coresponding entry in the array,
	// Call the contained handler on the message from the channel.
	d.Channels[dsw.Index].Handler(dsw.Target)
}
