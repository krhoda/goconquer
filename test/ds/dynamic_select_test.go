package dstest

import (
	"fmt"
	"testing"
	"time"

	"github.com/krhoda/goconquer/ds"
)

var (
	// my favorite var.
	unit interface{}

	lesserHeard, greaterHeard, unblockingHeard = false, false, false
	cHandHeard, cCloseHeard, pcCloseHeard      = false, false, false

	lesserClosed, greaterClosed, unblockingClosed = false, false, false
	cHandClosed, cCloseClosed, pcCloseClosed      = false, false, false

	lesserChannel, greaterChannel   = ds.ChannelEntry{}, ds.ChannelEntry{}
	unblockingChannel, cHandChannel = ds.ChannelEntry{}, ds.ChannelEntry{}
	cCloseChannel, pcCloseChannel   = ds.ChannelEntry{}, ds.ChannelEntry{}

	fullSet = []ds.ChannelEntry{
		lesserChannel,
		greaterChannel,
		unblockingChannel,
		cHandChannel,
		cCloseChannel,
		pcCloseChannel,
	}
)

func reset() {
	lesserHeard, greaterHeard, unblockingHeard = false, false, false
	cHandHeard, cCloseHeard, pcCloseHeard = false, false, false

	lesserClosed, greaterClosed, unblockingClosed = false, false, false
	cHandClosed, cCloseClosed, pcCloseClosed = false, false, false

	lesserChannel = ds.ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: ds.HandlerEntry{
			Func: func(i interface{}) {
				lesserHeard = true
				fmt.Println(i)
			},
			Blocking: true,
		},
		OnClose: ds.OnCloseEntry{
			Func: func() {
				lesserClosed = true
				fmt.Println("Lesser Channel Has Closed")
			},
			Blocking: true,
		},
	}

	greaterChannel = ds.ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: ds.HandlerEntry{
			Func: func(i interface{}) {
				greaterHeard = true
				fmt.Println(i)
			},
			Blocking: true,
			Priority: true,
		},
		OnClose: ds.OnCloseEntry{
			Func: func() {
				fmt.Println("Greater Channel Has Closed")
				greaterClosed = true
			},
			Blocking: true,
		},
	}

	unblockingChannel = ds.ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: ds.HandlerEntry{
			Func: func(i interface{}) {
				unblockingHeard = true
				fmt.Println(i)
			},
			Blocking: false,
		},
		OnClose: ds.OnCloseEntry{
			Func: func() {
				fmt.Println("Unblocking Channel Has Closed")
				unblockingClosed = true
			},
			Blocking: false,
		},
	}

	cHandChannel = ds.ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: ds.HandlerEntry{
			Func: func(i interface{}) {
				cHandHeard = true
				fmt.Println(i)
			},
			Blocking: false,
		},
		OnClose: ds.OnCloseEntry{
			Func: func() {
				fmt.Println("cHand Channel Has Closed")
				cHandClosed = true
			},
			Blocking: true,
		},
	}

	cCloseChannel = ds.ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: ds.HandlerEntry{
			Func: func(i interface{}) {
				cCloseHeard = true
				fmt.Println(i)
			},
			Blocking: true,
		},
		OnClose: ds.OnCloseEntry{
			Func: func() {
				fmt.Println("cClose Channel Has Closed")
				cCloseClosed = true
			},
			Blocking: false,
		},
	}

	pcCloseChannel = ds.ChannelEntry{
		Channel: make(chan interface{}, 5),
		Handler: ds.HandlerEntry{
			Func: func(i interface{}) {
				pcCloseHeard = true
				fmt.Println(i)
			},
			Blocking: true,
			Priority: true,
		},
		OnClose: ds.OnCloseEntry{
			Func: func() {
				fmt.Println("pcClose Channel Has Closed")
				pcCloseClosed = true
			},
			Blocking: false,
		},
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

	selectMgr := ds.NewDynamicSelect(ka, []ds.ChannelEntry{lesserChannel})

	selectMgr.Kill()
	selectMgr.Forever()

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

	selectMgr := ds.NewDynamicSelect(ka, []ds.ChannelEntry{greaterChannel})

	selectMgr.Kill()
	greaterChannel.Channel <- "This should not be heard."

	selectMgr.Forever()

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

// func TestPriorityOverRegularMessage(t *testing.T) {
// 	defer reset()

// 	killActionTest := false
// 	ka := func() {
// 		killActionTest = true
// 	}

// 	selectMgr := ds.NewDynamicSelect(ka, []ds.ChannelEntry{greaterChannel})

// 	go func() {
// 		lesserChannel.Channel <- "This should not be heard."
// 	}()

// 	go func() {
// 		greaterChannel.Channel <- "This should be heard"
// 	}()

// 	go selectMgr.Forever()

// 	time.Sleep(time.Second / 100)

// 	if !killActionTest {
// 		t.Errorf("Kill Action wasn't called!")
// 	}

// 	if greaterHeard {
// 		t.Errorf("Priority Channel was improperly read from")
// 	}

// 	if !greaterClosed {
// 		t.Errorf("Child listener did not clean up!")
// 	}
// }
