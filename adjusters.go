package main

import "time"

type nullAdjuster struct {
}

func (adjuster *nullAdjuster) adjust(response *Response, state *OPState) {
}

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
	adjuster.avgTime += response.latency
	if adjuster.movingCount == 5 {
		adjuster.movingTime = adjuster.avgTime / 5
		if response.err != nil || adjuster.movingTime >= config.badResponseTime {
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
