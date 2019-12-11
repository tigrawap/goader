package main

import (
	"github.com/tigrawap/goader/utils"
	"math/rand"
	"runtime"
)

type fullPayload struct {
	data []byte
}

func (p *fullPayload) Get() []byte {
	return p.data
}

func (p *fullPayload) GetFull() []byte {
	return p.data
}

func (p *fullPayload) GetLength() int64 {
	return int64(len(p.data))
}

func newFullPayload(data []byte) *fullPayload {
	return &fullPayload{data}
}

type randomPayload struct {
	data []byte
	min  int64
	max  int64
}

func (p *randomPayload) Get() []byte {
	return p.data[0:p.GetLength()]
}

func (p *randomPayload) GetFull() []byte {
	return p.data
}

func (p *randomPayload) GetLength() int64 {
	if p.max == p.min {
		return p.max
	}
	return p.min + rand.Int63n(p.max-p.min)
}

func newRandomPayload(data []byte, min int64) *randomPayload {
	max := int64(len(data))
	return &randomPayload{data, min, max}
}

type fairRandomPayload struct {
	data   []byte
	min    int64
	max    int64
	roller *utils.WeightedRoller
}

func (p *fairRandomPayload) Get() []byte {
	return p.data[0:p.GetLength()]
}

func (p *fairRandomPayload) GetFull() []byte {
	return p.data
}

func (p *fairRandomPayload) GetLength() int64 {
	return p.roller.Roll()
}

func newFairPayload(data []byte, min int64, buckets int) *fairRandomPayload {
	max := int64(len(data))
	roller := utils.NewWeightedRoller(min, max, int64(buckets))
	return &fairRandomPayload{
		data:   data,
		min:    min,
		max:    max,
		roller: roller,
	}
}

type scratchDataPayloadGetter struct {
	data [][]byte
}

func newScratchDataPayloadGetter(maxSize int) scratchDataPayloadGetter {
	numBuffers := utils.MinInt(runtime.NumCPU()/2, 4) // No more then 4 buffers
	if numBuffers == 0 {
		numBuffers = 1
	}
	s := scratchDataPayloadGetter{
		data: make([][]byte, numBuffers),
	}
	for i := 0; i < numBuffers; i++ {
		s.data[i] = make([]byte, maxSize)
	}
	return s
}

func (s scratchDataPayloadGetter) GetBuffer(size int64) []byte {
	buff := s.data[rand.Intn(len(s.data))]
	return buff[:size]
}
