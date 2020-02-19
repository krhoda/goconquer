package exbo

import (
	"fmt"
	"time"
)

type Opts struct {
	Min          time.Duration
	Max          time.Duration
	CooldownTick time.Duration
	CooldownSize time.Duration
}

type ExpoBackoffManager struct {
	Ready          chan struct{}
	alive          bool
	startReq       chan chan struct{}
	backoffGuard   chan struct{}
	currentBackOff time.Duration
	maxBackOff     time.Duration
	minBackOff     time.Duration
	cooldownTick   time.Duration
	cooldownSize   time.Duration
	firstReq       bool
	cooldown       chan struct{}
	done           chan struct{} // Kill Run.
	kill           chan struct{} // Kill Routines.
}

func NewExpoBackoffManager(opts Opts) (ex *ExpoBackoffManager, err error) {
	if opts.Min > opts.Max {
		err = fmt.Errorf("Incoherent args, Min was greater than Max")
		return
	}

	bg := make(chan struct{}, 1)
	r := make(chan struct{}, 1)

	bg <- struct{}{}

	ex = &ExpoBackoffManager{
		Ready:          r,
		alive:          true,
		startReq:       make(chan chan struct{}),
		backoffGuard:   bg,
		currentBackOff: opts.Min,
		minBackOff:     opts.Min,
		maxBackOff:     opts.Max,
		cooldownTick:   opts.CooldownTick,
		cooldownSize:   opts.CooldownSize,
		firstReq:       true,
		cooldown:       make(chan struct{}),
		done:           make(chan struct{}),
		kill:           make(chan struct{}),
	}

	return
}

func (ebm *ExpoBackoffManager) Run() {
	ebm.alive = true

	defer func() {
		ebm.alive = false
	}()

	go ebm.runCooldown()

	ebm.Ready <- struct{}{}
	for {
		select {
		case <-ebm.done:
			close(ebm.kill)
			return
		case sleepChan := <-ebm.startReq:
			go ebm.handleSleepChan(sleepChan, ebm.kill)
		case <-ebm.cooldown:
			if ebm.currentBackOff > ebm.minBackOff {
				<-ebm.backoffGuard
				ebm.currentBackOff = ebm.currentBackOff - ebm.cooldownSize
				if ebm.currentBackOff < ebm.minBackOff {
					ebm.currentBackOff = ebm.minBackOff
				}
				ebm.backoffGuard <- struct{}{}
			}
		}
	}
}

func (ebm *ExpoBackoffManager) runCooldown() {
	for {
		select {
		case <-ebm.done:
			return
		case <-time.After(ebm.cooldownTick):
			go func() {
				ebm.cooldown <- struct{}{}
			}()
		}
	}
}

func (ebm *ExpoBackoffManager) Stop() {
	close(ebm.done)
}

func (ebm *ExpoBackoffManager) handleSleepChan(sleepChan, kill chan struct{}) {
	defer close(sleepChan)

	<-ebm.backoffGuard
	timeout := ebm.currentBackOff
	ebm.currentBackOff = ebm.currentBackOff * 2
	if ebm.currentBackOff > ebm.maxBackOff {
		ebm.currentBackOff = ebm.maxBackOff
	}
	ebm.backoffGuard <- struct{}{}

	select {
	case <-kill:
		return
	case <-time.After(timeout):
		sleepChan <- struct{}{}
		return
	}
}

func (ebm *ExpoBackoffManager) Wait() error {
	if !ebm.alive {
		return fmt.Errorf("ebm recieved a kill command from the calling application, this is not the timeout returning")
	}

	select {
	case <-ebm.kill:
		return fmt.Errorf("ebm recieved a kill command from the calling application, this is not the timeout returning")

	default:
		x := make(chan struct{}, 1)
		ebm.startReq <- x
		_, ok := <-x
		if !ok {
			return fmt.Errorf("ebm recieved a kill command from the calling application, this is not the timeout returning")
		}

		return nil
	}

}

// CurrentWaitTime returns the current backoff wait time, if it is minimum, and if it is maximum.
func (ebm *ExpoBackoffManager) CurrentWaitTime() (time.Duration, bool, bool) {
	if !ebm.alive {
		return ebm.minBackOff, true, false
	}

	<-ebm.backoffGuard
	current := ebm.currentBackOff
	isMin := current == ebm.minBackOff
	isMax := current == ebm.maxBackOff
	ebm.backoffGuard <- struct{}{}

	return current, isMin, isMax
}
