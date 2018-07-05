package utils

import (
	"math/rand"
)

type weightedRoller struct {
	start         int64
	end           int64
	sortedWeights []int64
	stepStart     []int64
	step          int64
	totalWeight   int64
}

func (r *weightedRoller) Roll() int64 {
	roll := rand.Int63n(r.totalWeight)
	var i int
	for i = 0; i < len(r.sortedWeights); i++ { // TODO: Worth bisecting instead
		if r.sortedWeights[i] > roll {
			break
		}
	}
	start := r.stepStart[i]
	//return start
	end := start + r.step
	if end > r.end {
		end = r.end
	}
	rollDistance := end - start
	return start + rand.Int63n(rollDistance)
}

func NewWeightedRoller(minSize int64, maxSize int64, numBuckets int64) *weightedRoller {
	if maxSize-minSize < numBuckets {
		numBuckets = maxSize - minSize + 1
	}
	precisionMultiplier := numBuckets
	var i int64

	roller := weightedRoller{
		start: minSize,
		end:   maxSize,
		step:  (maxSize - minSize) / numBuckets,
	}

	for i = 0; i < numBuckets; i++ {
		start := minSize + i*roller.step
		end := start + roller.step
		if end > maxSize {
			end = maxSize
		}
		localWeight := (maxSize * precisionMultiplier) / (start + end)
		roller.totalWeight += localWeight
		roller.sortedWeights = append(roller.sortedWeights, roller.totalWeight)
		roller.stepStart = append(roller.stepStart, start)
	}
	return &roller
}
