package main

import (
	"math/rand"
	"time"
)

//Incremental target increases number from one, one by one
type IncrementalTarget struct {
	num     int64
	targets chan int64
}

func newIncrementalTarget() *IncrementalTarget {
	i := IncrementalTarget{}
	go func() {
		n := int64(0)
		i.targets = make(chan int64, 1000)
		for {
			n++
			i.targets <- n
		}
	}()
	return &i
}

func (i *IncrementalTarget) get() int64 {
	return <-i.targets
}

type BoundTarget struct {
	bound *[]int64
}

func (b *BoundTarget) get() int64 {
	for {
		nums := *b.bound
		if len(nums) == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		return nums[rand.Intn(int(len(nums)))]
	}
}
