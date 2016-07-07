package main

import (
	"fmt"
	"time"
)

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
		switch config.mode {
		case LowLatency:
			if response.err != nil || adjuster.movingTime >= badResponseTime*time.Millisecond {
				if state.speed > 1 {
					state.speed--
				}
			} else {
				state.speed++
			}
			fmt.Printf("Channel size adjusted to %v %v\n", state.speed, adjuster.movingTime)
			adjuster.movingCount = 0
			adjuster.avgTime = 0
		}
	}
}
