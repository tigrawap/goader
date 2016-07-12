package main

import (
	"time"
)

type threadedEmitter struct{}

func (emitter *threadedEmitter) emitRequests(state *OPState) {
	for state.speed > state.inFlight {
		state.requests <- 1
		state.inFlight++
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
	return emitter
}

type rateEmitter struct {
	startedAt    time.Time
	emitEvery    time.Duration
	totalEmitted int64
}

func (emitter *rateEmitter) emitRequests(state *OPState) {
	if emitter.emitEvery == 0 {
		return
	}

	totalTimePassed := time.Since(emitter.startedAt)
	shouldEmit := int64(totalTimePassed / emitter.emitEvery)
	notEmitted := shouldEmit - emitter.totalEmitted
	var i int64
	for i = 0; i < notEmitted; i++ {
		state.inFlight++
		emitter.totalEmitted++
		state.requests <- 1
	}
}
