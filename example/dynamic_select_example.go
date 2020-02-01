package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/krhoda/goconquer/gcq"
)

func trivial(i interface{}) {
	fmt.Println(i)
}

func main() {
	k := make(chan os.Signal, 1)
	ka := func() {
		log.Fatal("IT'S ALL OVER!")
	}

	ch1 := make(chan interface{})
	ch2 := make(chan interface{})

	chSl := []gcq.ChannelEntry{
		{
			Channel: ch1,
			Handler: trivial,
		},
		{
			Channel: ch2,
			Handler: trivial,
		},
	}

	ds := gcq.DynamicSelect{
		Kill:         k,
		OnKillAction: ka,
		Channels:     chSl,
	}

	go func() {
		ch1 <- 22
		ch2 <- 77
		log.Println("ALL DONE!")

		time.Sleep(time.Second * 10)
	}()

	ds.Forever()
}
