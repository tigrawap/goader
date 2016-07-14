package main

import (
	"math/rand"
	"time"
)

//IncrementalTarget increases number of request one by one
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

// BoundTarget will set number of requests randomaly selected from bound slice
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
