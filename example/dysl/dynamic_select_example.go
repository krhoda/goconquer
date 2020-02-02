package main

import (
	"log"
	"os"
	"time"

	"github.com/krhoda/goconquer/dysl"
	"github.com/krhoda/goconquer/example/helpers/bots"
)

func main() {
	ka := func() {
		log.Println("IT'S ALL OVER!")
	}

	ch1 := make(chan interface{})
	ch2 := make(chan interface{})

	chSl := []dysl.ChannelEntry{
		{
			Channel: ch1,
			Handler: dysl.HandlerEntry{
				Func:     bots.HandleStringBot,
				Blocking: true,
			},
			OnClose: dysl.OnCloseEntry{
				Func:     func() {},
				Blocking: true,
			},
		},
		{
			Channel: ch2,
			Handler: dysl.HandlerEntry{
				Func:     bots.HandleMathBot,
				Blocking: true,
			},
			OnClose: dysl.OnCloseEntry{
				Func:     func() {},
				Blocking: true,
			},
		},
	}

	ds := dysl.NewDynamicSelect(ka, chSl)

	go func() {
		k := make(chan os.Signal, 1)
		<-k
		close(ds.Kill)
	}()

	go bots.MakeStringBot(ch1, ds.Kill)
	go bots.MakeMathBot(ch2, ds.Kill)

	go ds.Forever()

	time.Sleep(time.Second * 10)
	log.Println("Main thread ending action...")
	close(ds.Kill)
	time.Sleep(time.Second * 5)
	log.Println("...Main thread exiting")
}
