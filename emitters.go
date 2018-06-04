package main

import (
	"sync/atomic"
	"time"
)

type threadedEmitter struct{}

func (emitter *threadedEmitter) emitRequests(state *OPState) {
	if state.speed <= 0 {
		return
	}
	var inFlight int32
	state.inFlightCallback = make(chan int32, 1000)
MAINLOOP:
	for {
		if inFlight >= int32(state.speed) {
		FOR:
			for {
				select {
				case inFlight = <-state.inFlightCallback:
				default:
					if inFlight >= int32(state.speed) {
						inFlight = <-state.inFlightCallback
						continue MAINLOOP
					}
					break FOR
				}
			}
		}
		inFlight = atomic.AddInt32(&state.inFlight, 1)
		state.requests <- 1
	}
}

type boundEmitter struct {
	boundTo *int64
	boundBy *float64
	emitted int64
}

func (e *boundEmitter) emitRequests(state *OPState) {
	var shouldEmitted float64
	for {
		shouldEmitted = float64(*e.boundTo) * *e.boundBy
		for int64(shouldEmitted) > e.emitted {
			e.emitted++
			atomic.AddInt32(&state.inFlight, 1)
			state.requests <- 1
		}
		time.Sleep(time.Millisecond)
	}
}

func newRateEmitter(perSecond int) *rateEmitter {
	var emitEvery time.Duration
	if perSecond != 0 {
		emitEvery = time.Duration(time.Second) / time.Duration(perSecond)
	} else {
		emitEvery = 0
	}
	emitter := new(rateEmitter)
	emitter.startedAt = time.Now()
	emitter.emitEvery = emitEvery
	emitter.unlimiter = make(chan *OPState, perSecond+1)
	go func() {
		var state *OPState
		for {
			state = <-emitter.unlimiter
			state.requests <- 1
		}
	}()
	return emitter
}

type rateEmitter struct {
	startedAt    time.Time
	emitEvery    time.Duration
	perSecond    int
	totalEmitted int
	unlimiter    chan *OPState
}

func (emitter *rateEmitter) sendRequest(state *OPState) {
	emitter.unlimiter <- state
}

func (emitter *rateEmitter) emitRequests(state *OPState) {
	if emitter.emitEvery == 0 {
		return
	}
	sleepFor := emitter.emitEvery
	if sleepFor < time.Millisecond {
		sleepFor = time.Millisecond
	}

	for {
		totalTimePassed := time.Since(emitter.startedAt)
		shouldEmit := int(totalTimePassed / emitter.emitEvery)
		notEmitted := shouldEmit - emitter.totalEmitted
		atomic.AddInt32(&state.inFlight, int32(notEmitted))
		emitter.totalEmitted += notEmitted
		for i := 0; i < notEmitted; i++ {
			emitter.sendRequest(state)
		}
		time.Sleep(sleepFor)
	}
}
