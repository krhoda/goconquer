package exbo

import (
	"log"
	"sync"
	"testing"
	"time"
)

var testSlowOpts = Opts{
	Min:          time.Hour,
	Max:          time.Hour * 2,
	CooldownTick: time.Hour * 2,
	CooldownSize: time.Minute / 2,
}

var testUpOpts = Opts{
	Min:          time.Second,
	Max:          time.Minute,
	CooldownTick: time.Hour * 2,
	CooldownSize: time.Second,
}

var testDownOpts = Opts{
	Min:          time.Second,
	Max:          time.Second * 10,
	CooldownTick: time.Second * 3,
	CooldownSize: time.Second * 5,
}

func TestNew(t *testing.T) {
	badOpts := Opts{
		Min:          time.Hour,
		Max:          time.Second,
		CooldownTick: time.Hour / 2,
		CooldownSize: time.Second,
	}

	_, err := NewExpoBackoffManager(badOpts)
	if err == nil {
		t.Errorf("Bad opts were excepted")
	}

	_, err = NewExpoBackoffManager(testSlowOpts)
	if err != nil {
		t.Errorf("Good opts were rejected")
	}
}

func TestWaitAndStop(t *testing.T) {
	ex, err := NewExpoBackoffManager(testSlowOpts)
	if err != nil {
		t.Errorf("Good opts were rejected")
	}

	x := make(chan struct{})

	go ex.Run()
	<-ex.Ready

	go func() {
		defer close(x)
		x <- struct{}{}
		err := ex.Wait()
		if err == nil {
			t.Errorf("Did not hear forced error")
		}
	}()

	<-x
	ex.Stop()
	<-x

}

func TestIncreaseBackoff(t *testing.T) {
	ex, err := NewExpoBackoffManager(testUpOpts)
	if err != nil {
		t.Errorf("Good opts were rejected")
	}

	go ex.Run()
	<-ex.Ready

	current, isMin, isMax := ex.CurrentWaitTime()
	if current != testUpOpts.Min {
		t.Errorf("Current wait time does not equal expected min wait time from the options")
	}

	if isMax {
		t.Errorf("isMax boolean is incorrectly set to true")
	}

	if !isMin {
		t.Errorf("isMin boolean is incorrectly set to false")
	}

	var wg sync.WaitGroup

	wg.Add(60)

	for i := 0; i < 60; i++ {
		go func() {
			wg.Done()
			ex.Wait()
		}()
	}

	wg.Wait()
	log.Println("About to sleep for 1 second buffer...")
	time.Sleep(time.Second)

	current, isMin, isMax = ex.CurrentWaitTime()
	if current != testUpOpts.Max {
		t.Errorf("Current wait time does not equal expected wait time from the options")
	}

	if !isMax {
		t.Errorf("isMax boolean is incorrectly set to false")
	}

	if isMin {
		t.Errorf("isMin boolean is incorrectly set to true")
	}

	ex.Stop()
}

func TestCooldown(t *testing.T) {
	ex, err := NewExpoBackoffManager(testDownOpts)
	if err != nil {
		t.Errorf("Good opts were rejected")
	}

	go ex.Run()
	<-ex.Ready

	current, isMin, isMax := ex.CurrentWaitTime()
	if current != testDownOpts.Min {
		t.Errorf("Current wait time does not equal expected min wait time from the options")
	}

	if isMax {
		t.Errorf("isMax boolean is incorrectly set to true")
	}

	if !isMin {
		t.Errorf("isMin boolean is incorrectly set to false")
	}

	var wg sync.WaitGroup

	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			wg.Done()
			ex.Wait()
		}()
	}

	wg.Wait()

	current, isMin, isMax = ex.CurrentWaitTime()
	if current != testDownOpts.Max {
		t.Errorf("Current wait time does not equal expected wait time from the options")
	}

	if !isMax {
		t.Errorf("isMax boolean is incorrectly set to false")
	}

	if isMin {
		t.Errorf("isMin boolean is incorrectly set to true")
	}

	log.Println("About to sleep for 7 seconds to allow a 6s timer to run...")
	time.Sleep(time.Second * 7)

	current, isMin, isMax = ex.CurrentWaitTime()
	if current != testDownOpts.Min {
		t.Errorf("Current wait time does not equal expected min wait time from the options")
	}

	if isMax {
		t.Errorf("isMax boolean is incorrectly set to true")
	}

	if !isMin {
		t.Errorf("isMin boolean is incorrectly set to false")
	}

	ex.Stop()
}
