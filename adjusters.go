package main

import (
	"math"
	"time"
)

type nullAdjuster struct {
}

func (adjuster *nullAdjuster) adjust(response *Response) {
}

var logbase = 1 / math.Log(1.3)

type latencyAdjuster struct {
	movingCount int
	avgTime     time.Duration
	movingTime  time.Duration
	state       *OPState
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
const thresholdPercent = 5

func newLatencyAdjuster(state *OPState) *latencyAdjuster {
	a := latencyAdjuster{state: state}
	return &a
}

func (a *latencyAdjuster) adjust(response *Response) {
	a.movingCount++
	sample := int(math.Log(float64(a.state.speed)) * logbase)
	if response.err != nil {
		a.avgTime += config.maxLatency * time.Duration(sample)
	} else {
		a.avgTime += response.latency
	}
	if a.movingCount >= sample {
		a.movingTime = a.avgTime / time.Duration(a.movingCount)
		if a.movingTime >= config.maxLatency/100*(100+thresholdPercent) {
			if a.state.speed > 1 {
				p(a.state.colored(arrowDown))
				a.state.speed--
			}
		} else if a.movingTime <= config.maxLatency/100*(100-thresholdPercent) {
			if a.state.speed < config.maxChannels {
				p(a.state.colored(arrowUp))
				a.state.speed++
			}
		}
		a.movingCount = 0
		a.avgTime = 0
	}
}
