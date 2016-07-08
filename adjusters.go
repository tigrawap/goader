package main

import "time"

type nullAdjuster struct {
}

func (adjuster *nullAdjuster) adjust(response *Response, state *OPState) {
}

type speedAdjuster struct {
	movingCount int
	avgTime     time.Duration
	movingTime  time.Duration
}

func (adjuster *speedAdjuster) adjust(response *Response, state *OPState) {
	adjuster.movingCount++
	adjuster.avgTime += response.latency
	if adjuster.movingCount == 5 {
		adjuster.movingTime = adjuster.avgTime / 5
		if response.err != nil || adjuster.movingTime >= config.badResponseTime {
			if state.speed > 1 {
				state.speed--
			}
		} else {
			state.speed++
		}
		adjuster.movingCount = 0
		adjuster.avgTime = 0
	}
}
