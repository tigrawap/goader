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
	a.state.speed = *a.boundTo * *a.boundBy
	if a.state.speed > config.maxChannels {
		a.state.speed = config.maxChannels
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

func (a *latencyAdjuster) adjust(response *Response) {
	a.movingCount++
	barrierPenalty := (a.state.speed + 1) - a.barrier
	sample := int(math.Log(float64(a.state.speed)) * logbase)
	if barrierPenalty > 0 && a.barrier > 0 {
		sample += a.state.speed * barrierPenalty
		// Making it hard to breach the minimal speed with errors
	}

	if response.err != nil {
		if a.barrier > a.state.speed || a.barrier == 0 {
			a.barrier = a.state.speed
			p(fmt.Sprintf("[%d]", a.barrier))
		}
		a.movingTotalTime += config.maxLatency * time.Duration(sample) // Forcing to drop on error
		sample = 1
	} else {
		a.movingTotalTime += response.latency
	}
	if a.movingCount >= sample {
		a.movingAvgTime = a.movingTotalTime / time.Duration(a.movingCount)
		if a.movingAvgTime >= config.maxLatency/100*(100+thresholdPercent) {
			if a.state.speed > 1 {
				p(a.state.colored(arrowDown))
				if a.state.speed > 1 {
					a.state.speed--
					a.movingCount =0
					a.movingTotalTime = 0
				}
			}
		} else if a.movingAvgTime <= config.maxLatency/100*(100-thresholdPercent) {
			if a.state.speed < config.maxChannels {
				p(a.state.colored(arrowUp))
				a.state.speed++
				if a.barrier != 0 {
					if a.state.speed - a.barrier > 3 {
						a.barrier++
					}
				}
				a.movingCount = 0
				a.movingTotalTime = 0
			}
		}
	}
}
