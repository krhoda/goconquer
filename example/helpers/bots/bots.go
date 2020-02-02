package bots

import (
	"log"
	"math/rand"
	"time"
)

func MakeStringBot(send, done chan interface{}) {
	defer close(send)

	quips := []string{
		"...History teaches us we do not learn from history",
		"If traveling salesrep is visiting twenty cities and....",
		"...they must cross seven bridges in Kroneburg but only...",
		"...Lorus impsum...",
	}

	for {
		select {
		case <-done:
			return
		case <-time.After(time.Second * 5):
			log.Println("Going To Send In String Bot!")
			quip := quips[rand.Intn(len(quips))]
			send <- quip
		}
	}

}

func HandleStringBot(i interface{}) {
	x, ok := i.(string)
	if !ok {
		log.Printf("I don't know what String bot said:\n%v\n", i)
		return
	}

	log.Printf("String bot is saying something odd... %s\n", x)
}

func MakeMathBot(send, done chan interface{}) {
	defer close(send)

	quips := []float64{
		3.14,
		1.62,
		6.03,
		2.79,
		9.81,
	}

	for {
		select {
		case <-done:
			return
		case <-time.After(time.Second * 2):
			log.Println("Going To Send In Math Bot!")
			quip := quips[rand.Intn(len(quips))]
			send <- quip
		}
	}

}

func HandleMathBot(i interface{}) {
	x, ok := i.(float64)
	if !ok {
		log.Printf("I don't know what math bot said:\n%v\n", i)
		return
	}

	switch x {
	case 3.14:
		log.Printf("Math bot is a big fan of pastries: %f\n", x)
	case 1.62:
		log.Printf("Math bot says you're golden: %f\n", x)
	case 6.03:
		log.Printf("Math bot is molling something over: %f\n", x)
	case 2.72:
		log.Printf("Math bot says Euler is a ruler %f\n", x)
	case 9.81:
		log.Printf("Math bot says watch your step: %f\n", x)
	default:
		log.Printf("Math bot learned a new number: %f\nHorrifying.\n", x)
	}
}
