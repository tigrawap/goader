package main

import (
	"fmt"
	"math"
	"time"
)

type nullAdjuster struct {
}

func (adjuster *nullAdjuster) adjust(response *Response) {
}

type latencyAdjuster struct {
	movingCount     int
	movingTotalTime time.Duration
	movingAvgTime   time.Duration
	state           *OPState
	barrier         int
}

type boundAdjuster struct {
	boundTo *int
	boundBy *int
	state   *OPState
}

func (a *boundAdjuster) adjust(response *Response) {
	a.state.concurrency = *a.boundTo * *a.boundBy
	if a.state.concurrency > config.maxChannels {
		a.state.concurrency = config.maxChannels
	}
	return
}

const arrowUp = `↗`
const arrowDown = `↘`

func newLatencyAdjuster(state *OPState) *latencyAdjuster {
	a := latencyAdjuster{state: state}
	return &a
}

var logbase = 1 / math.Log(1.3)

const thresholdPercent = 5

func (a *latencyAdjuster) decrease() {
	if a.state.concurrency > 1 {
		p(a.state.colored(arrowDown))
		a.state.concurrency--
		a.movingCount = 0
		a.movingTotalTime = 0
	}
}

func (a *latencyAdjuster) increase() {
	p(a.state.colored(arrowUp))
	a.state.concurrency++
	a.movingCount = 0
	a.movingTotalTime = 0
}

func (a *latencyAdjuster) setBarrier(barrier int) {
	a.barrier = barrier
	p(fmt.Sprintf("[%d]", a.barrier))
}

func (a *latencyAdjuster) adjust(response *Response) {
	a.movingCount++
	barrierPenalty := (a.state.concurrency + 1) - a.barrier
	sample := int(math.Log(float64(a.state.concurrency)) * logbase)
	if barrierPenalty > 0 && a.barrier > 0 {
		sample += a.state.concurrency * barrierPenalty
	}

	if response.err != nil {
		if a.barrier > a.state.concurrency || a.barrier == 0 {
			a.setBarrier(a.state.concurrency)
		}
		a.decrease()
		return
	} else {
		a.movingTotalTime += response.latency
	}

	if a.movingCount >= sample {
		a.movingAvgTime = a.movingTotalTime / time.Duration(a.movingCount)
		if a.movingAvgTime >= config.maxLatency/100*(100+thresholdPercent) {
			a.decrease()
		} else if a.movingAvgTime <= config.maxLatency/100*(100-thresholdPercent) {
			if a.state.concurrency < config.maxChannels {
				a.increase()
				if a.barrier != 0 {
					if a.state.concurrency-a.barrier > 4 {
						a.setBarrier(0)
					}
				}
			}
		}
	}
}
