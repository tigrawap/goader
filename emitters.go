package main

import (
	"math/rand"
	"time"
)

type threadedEmitter struct{}

func (emitter *threadedEmitter) emitRequests(channels *OPChannels, state *OPState) {
	for state.speed > state.inFlight {
		state.num++
		channels.requests <- &Request{state.num}
		state.inFlight++
	}
}

func newRateEmitter(perSecond int, allowedNums *[]int64, useAllowed bool) *rateEmitter {
	var emitEvery time.Duration
	if perSecond != 0 {
		emitEvery = time.Duration(time.Second) / time.Duration(perSecond)
	} else {
		emitEvery = 0
	}
	emitter := new(rateEmitter)
	if allowedNums != nil {
		emitter.allowedNums = allowedNums
	}
	emitter.startedAt = time.Now()
	emitter.emitEvery = emitEvery
	emitter.useAllowed = useAllowed
	return emitter
}

type rateEmitter struct {
	startedAt    time.Time
	emitEvery    time.Duration
	totalEmitted int64
	allowedNums  *[]int64
	useAllowed   bool
}

func (emitter *rateEmitter) emitRequests(channels *OPChannels, state *OPState) {
	if emitter.emitEvery == 0 {
		return
	}

	totalTimePassed := time.Since(emitter.startedAt)
	shouldEmit := int64(totalTimePassed / emitter.emitEvery)
	notEmitted := shouldEmit - emitter.totalEmitted
	var i int64
	for i = 0; i < notEmitted; i++ {
		state.num++
		var num int64
		if emitter.useAllowed {
			nums := *emitter.allowedNums
			if len(nums) == 0 {
				break
			}
			num = nums[rand.Intn(int(len(nums)))]
		} else {
			num = state.num
		}
		state.inFlight++
		emitter.totalEmitted++
		channels.requests <- &Request{num}
	}
}
