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
		"...they must cross seven bridges in Konigsburg but only...",
		"...Lorus impsum...",
	}

	for {
		select {
		case <-done:
			return
		case <-time.After(time.Second * time.Duration(rand.Intn(5))):
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

const myPi = 3.14
const myGolden = 1.62
const myAvo = 6.03
const myEuler = 2.72
const myGrav = 9.81

func MakeMathBot(send, done chan interface{}) {
	defer close(send)

	quips := []float64{
		myPi,
		myGolden,
		myAvo,
		myEuler,
		myGrav,
	}

	for {
		select {
		case <-done:
			return
		case <-time.After(time.Second * time.Duration(rand.Intn(5))):
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
	case myPi:
		log.Printf("Math bot is a big fan of pastries: %f\n", x)
	case myGolden:
		log.Printf("Math bot says you're golden: %f\n", x)
	case myAvo:
		log.Printf("Math bot is molling something over: %f\n", x)
	case myEuler:
		log.Printf("Math bot says Euler is a ruler %f\n", x)
	case myGrav:
		log.Printf("Math bot says watch your step: %f\n", x)
	default:
		log.Printf("Math bot learned a new number: %f\nHorrifying.\n", x)
	}
}

func MakeRuneBot(send, done chan interface{}) {
	defer close(send)

	letterRunes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	for {
		select {
		case <-done:
			return
		case <-time.After(time.Second * time.Duration(rand.Intn(5))):
			log.Println("Going To Send In Rune Bot!")
			b := letterRunes[rand.Intn(len(letterRunes))]
			send <- b
		}
	}
}

func HandleRuneBot(i interface{}) {
	x, ok := i.(rune)
	if !ok {
		log.Printf("I don't know what rune bot said:\n%v\n", i)
		return
	}

	log.Printf("Rune bot said '%s'\nFacinating...\n", string(x))
}
