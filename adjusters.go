package main

import (
	"math"
	"time"
)

type nullAdjuster struct {
}

func (adjuster *nullAdjuster) adjust(response *Response, state *OPState) {
}

var logbase = 1 / math.Log(1.3)

type latencyAdjuster struct {
	movingCount int
	avgTime     time.Duration
	movingTime  time.Duration
}

type boundAdjuster struct {
	boundTo *int
	boundBy *int
}

func (adjuster *boundAdjuster) adjust(response *Response, state *OPState) {
	state.speed = *adjuster.boundTo * *adjuster.boundBy
	if state.speed > config.maxChannels {
		state.speed = config.maxChannels
	}
	return
}

func (adjuster *latencyAdjuster) adjust(response *Response, state *OPState) {
	adjuster.movingCount++
	sample := int(math.Log(float64(state.speed)) * logbase)
	if response.err != nil {
		adjuster.avgTime += config.badResponseTime * time.Duration(sample)
	} else {
		adjuster.avgTime += response.latency
	}
	if adjuster.movingCount >= sample {
		adjuster.movingTime = adjuster.avgTime / time.Duration(adjuster.movingCount)
		if adjuster.movingTime >= config.badResponseTime {
			if state.speed > 1 {
				state.speed--
			}
		} else {
			if state.speed < config.maxChannels {
				state.speed++
			}
		}
		adjuster.movingCount = 0
		adjuster.avgTime = 0
	}
}
