package ds

import (
	"fmt"
	"testing"
	"time"
)

var (
	ready                                      = make(chan interface{})
	lesserHeard, greaterHeard, unblockingHeard = false, false, false
	cHandHeard, cCloseHeard, pcCloseHeard      = false, false, false

	lesserClosed, greaterClosed, unblockingClosed = false, false, false
	cHandClosed, cCloseClosed, pcCloseClosed      = false, false, false

	lesserChannel, greaterChannel   = ChannelEntry{}, ChannelEntry{}
	unblockingChannel, cHandChannel = ChannelEntry{}, ChannelEntry{}
	cCloseChannel, pcCloseChannel   = ChannelEntry{}, ChannelEntry{}

	fullSet = []ChannelEntry{
		lesserChannel,
		greaterChannel,
		unblockingChannel,
		cHandChannel,
		cCloseChannel,
		pcCloseChannel,
	}
)

func reset() {
	ready = make(chan interface{})
	lesserHeard, greaterHeard, unblockingHeard = false, false, false
	cHandHeard, cCloseHeard, pcCloseHeard = false, false, false

	lesserClosed, greaterClosed, unblockingClosed = false, false, false
	cHandClosed, cCloseClosed, pcCloseClosed = false, false, false

	lesserChannel = ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: HandlerEntry{
			Func: func(i interface{}) {
				lesserHeard = true
				fmt.Println(i)
			},
			Blocking: true,
		},
		OnClose: OnCloseEntry{
			Func: func() {
				lesserClosed = true
				fmt.Println("Lesser Channel Has Closed")
			},
			Blocking: true,
		},
	}

	greaterChannel = ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: HandlerEntry{
			Func: func(i interface{}) {
				greaterHeard = true
				fmt.Println(i)
			},
			Blocking: true,
			Priority: true,
		},
		OnClose: OnCloseEntry{
			Func: func() {
				fmt.Println("Greater Channel Has Closed")
				greaterClosed = true
			},
			Blocking: true,
		},
	}

	unblockingChannel = ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: HandlerEntry{
			Func: func(i interface{}) {
				unblockingHeard = true
				fmt.Println(i)
			},
			Blocking: false,
		},
		OnClose: OnCloseEntry{
			Func: func() {
				fmt.Println("Unblocking Channel Has Closed")
				unblockingClosed = true
			},
			Blocking: false,
		},
	}

	cHandChannel = ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: HandlerEntry{
			Func: func(i interface{}) {
				cHandHeard = true
				fmt.Println(i)
			},
			Blocking: false,
		},
		OnClose: OnCloseEntry{
			Func: func() {
				fmt.Println("cHand Channel Has Closed")
				cHandClosed = true
			},
			Blocking: true,
		},
	}

	cCloseChannel = ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: HandlerEntry{
			Func: func(i interface{}) {
				cCloseHeard = true
				fmt.Println(i)
			},
			Blocking: true,
		},
		OnClose: OnCloseEntry{
			Func: func() {
				fmt.Println("cClose Channel Has Closed")
				cCloseClosed = true
			},
			Blocking: false,
		},
	}

	pcCloseChannel = ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: HandlerEntry{
			Func: func(i interface{}) {
				pcCloseHeard = true
				fmt.Println(i)
			},
			Blocking: true,
			Priority: true,
		},
		OnClose: OnCloseEntry{
			Func: func() {
				fmt.Println("pcClose Channel Has Closed")
				pcCloseClosed = true
			},
			Blocking: false,
		},
	}

	fullSet = []ChannelEntry{
		lesserChannel,
		greaterChannel,
		unblockingChannel,
		cHandChannel,
		cCloseChannel,
		pcCloseChannel,
	}
}

func init() {
	reset()
}

func TestKill(t *testing.T) {
	defer reset()

	killActionTest := false
	ka := func() {
		killActionTest = true
	}

	selectMgr := NewDynamicSelect(ka, []ChannelEntry{lesserChannel})

	selectMgr.Kill()
	selectMgr.Forever(ready)

	if !killActionTest {
		t.Errorf("Kill Action wasn't called!")
	}

	time.Sleep(time.Second / 100)
	if !lesserClosed {
		t.Errorf("Child listener did not clean up!")
	}
}

func TestKillOverPriorityMessage(t *testing.T) {
	defer reset()

	killActionTest := false
	ka := func() {
		killActionTest = true
	}

	selectMgr := NewDynamicSelect(ka, []ChannelEntry{greaterChannel})

	selectMgr.Kill()
	greaterChannel.Channel <- "This should not be heard."

	selectMgr.Forever(ready)

	time.Sleep(time.Second / 100)

	if !killActionTest {
		t.Errorf("Kill Action wasn't called!")
	}

	if greaterHeard {
		t.Errorf("Priority Channel was improperly read from")
	}

	if !greaterClosed {
		t.Errorf("Child listener did not clean up!")
	}
}

func TestLoad(t *testing.T) {
	defer reset()

	killActionTest := false
	ka := func() {
		killActionTest = true
	}

	selectMgr := NewDynamicSelect(ka, []ChannelEntry{lesserChannel})
	err := selectMgr.Load(unblockingChannel)
	if err == nil {
		t.Errorf("Load err was nil when it should not have been.")
	}

	go selectMgr.Forever(ready)
	<-ready

	lesserChannel.Channel <- unit

	err = selectMgr.Load(unblockingChannel)
	if err != nil {
		t.Errorf("Could not load when expected to: %s", err.Error())
	}

	unblockingChannel.Channel <- unit
	time.Sleep(time.Second / 10)

	selectMgr.Kill()
	time.Sleep(time.Second / 10)

	err = selectMgr.Load(greaterChannel)
	if err == nil {
		t.Errorf("Load err was nil when it should not have been.")
	}

	if !killActionTest {
		t.Errorf("Kill Action wasn't called!")
	}

	if !lesserHeard {
		t.Errorf("Lesser was not heard.")
	}

	if !unblockingHeard {
		t.Errorf("Unblocking was not heard.")
	}

	if !unblockingClosed {
		t.Errorf("Child listener did not clean up!")
	}

	if !lesserClosed {
		t.Errorf("Child listener did not clean up!")
	}
}

func TestIsAlive(t *testing.T) {
	defer reset()

	ka := func() {}

	selectMgr := NewDynamicSelect(ka, []ChannelEntry{lesserChannel})
	if !selectMgr.IsAlive() {
		t.Errorf("DynamicSelect improperly stating status! Says dead instead of alive")
	}

	go selectMgr.Forever(ready)

	time.Sleep(time.Second / 10)
	if !selectMgr.IsAlive() {
		t.Errorf("DynamicSelect improperly stating status! Says dead instead of alive")
	}

	selectMgr.Kill()

	if selectMgr.IsAlive() {
		t.Errorf("DynamicSelect improperly stating status! Says alive instead of dead")
	}

	time.Sleep(time.Second / 10)
	if !lesserClosed {
		t.Errorf("Child listener did not clean up!")
	}
}

func TestAllChannelTypes(t *testing.T) {
	defer reset()

	ka := func() {}

	selectMgr := NewDynamicSelect(ka, fullSet)

	go selectMgr.Forever(ready)
	<-ready

	for _, v := range fullSet {
		v.Channel <- unit
	}

	time.Sleep(time.Second / 10)
	selectMgr.Kill()

	time.Sleep(time.Second / 10)

	if !lesserHeard {
		t.Errorf("Lesser was not heard.")
	}

	if !unblockingHeard {
		t.Errorf("Unblocking was not heard.")
	}

	if !greaterHeard {
		t.Errorf("Lesser was not heard.")
	}

	if !cCloseHeard {
		t.Errorf("Unblocking was not heard.")
	}

	if !cHandHeard {
		t.Errorf("Lesser was not heard.")
	}

	if !pcCloseHeard {
		t.Errorf("Unblocking was not heard.")
	}

	if !unblockingClosed {
		t.Errorf("Child listener did not clean up!")
	}

	if !lesserClosed {
		t.Errorf("Child listener did not clean up!")
	}

	if !greaterClosed {
		t.Errorf("Child listener did not clean up!")
	}

	if !pcCloseClosed {
		t.Errorf("Child listener did not clean up!")
	}

	if !cCloseClosed {
		t.Errorf("Child listener did not clean up!")
	}

	if !cHandClosed {
		t.Errorf("Child listener did not clean up!")
	}
}
