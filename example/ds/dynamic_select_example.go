package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/krhoda/goconquer/ds"
	"github.com/krhoda/goconquer/example/helpers/bots"
)

func main() {
	ka := func() {
		log.Println("IT'S ALL OVER!")
	}

	ch1 := make(chan interface{})
	ch2 := make(chan interface{})

	chSl := []ds.ChannelEntry{
		{
			Channel: ch1,
			Handler: ds.HandlerEntry{
				Func:     bots.HandleStringBot,
				Blocking: false,
			},
			OnClose: ds.OnCloseEntry{
				Func:     func() { fmt.Println("Closing String bot!") },
				Blocking: false,
			},
		},
		{
			Channel: ch2,
			Handler: ds.HandlerEntry{
				Func:     bots.HandleMathBot,
				Blocking: true,
			},
			OnClose: ds.OnCloseEntry{
				Func:     func() { fmt.Println("Closing math bot!") },
				Blocking: false,
			},
		},
	}

	sMgr := ds.NewDynamicSelect(ka, chSl)

	go func() {
		k := make(chan os.Signal, 1)
		<-k
		close(sMgr.Kill)
	}()

	go bots.MakeStringBot(ch1, sMgr.Kill)
	go bots.MakeMathBot(ch2, sMgr.Kill)

	go sMgr.Forever()

	time.Sleep(time.Second * 5)
	log.Println("Main thread building Rune Bot...")
	ch3 := make(chan interface{})
	ce3 := ds.ChannelEntry{
		Channel: ch3,
		Handler: ds.HandlerEntry{
			Func:     bots.HandleRuneBot,
			Blocking: true,
			Priority: true,
		},
		OnClose: ds.OnCloseEntry{
			Func:     func() { fmt.Println("Closing RUNE bot!") },
			Blocking: true,
		},
	}

	go func() {
		err := sMgr.Load(ce3)
		if err != nil {
			log.Println("Error in Load: %s\n", err)
		}
	}()

	go bots.MakeRuneBot(ch3, sMgr.Kill)

	log.Println("Main thread has dispatch load message and rune bot...")
	time.Sleep(time.Second * 30)

	log.Println("...Main thread turning off other sevices...")
	close(sMgr.Kill)

	time.Sleep(time.Second * 5)
	log.Println("...Main thread exiting, other services off.")
}
