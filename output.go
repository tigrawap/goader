package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/mgutz/ansi"
)

//HumanOutput will represent results in human form
type HumanOutput struct {
	pb   chan string
	quit chan bool
}

//JSONOutput will represent results in json form
type JSONOutput struct{}

func newHumanOutput() *HumanOutput {
	o := HumanOutput{
		pb:   make(chan string, 1000),
		quit: make(chan bool),
	}
	go o.printer()
	return &o

}

func (o *HumanOutput) printer() {
	var done bool
	for {
		select {
		case <-o.quit:
			if !done{
				fmt.Println()
				done = true
			}
		case s := <-o.pb:
			if !done{
				fmt.Print(s)
			}
		}
	}
}

func (o *HumanOutput) progress(s string) {
	o.pb <- s
}

func (o *HumanOutput) printOpResult(r *OPResults, header string) {
	if r.Done == 0 {
		return
	}
	fmt.Println("\n", ansi.Color(header, "blue+h"))
	if r.Done-r.Errors > 0 {
		fmt.Println("Average response time:", r.AverageSpeed)
		var keys []int
		for k := range r.Percentiles {
			i, err := strconv.Atoi(k)
			if err == nil {
				keys = append(keys, i)
			}
		}
		sort.Ints(keys)
		for _, k := range keys {
			fmt.Println("Percentile", k, "-", r.Percentiles[strconv.Itoa(k)])
		}
		fmt.Println("TopTen latencies: ", TimesList(r.TopTen))

		if config.mode == LowLatency {
			fmt.Printf("Threads with latency below %v: %v\n", config.maxLatency, r.FinalSpeed)
		}
	}

	fmt.Println("Total requests:", r.Done)
	fmt.Println("Total errors:", r.Errors)
	if config.mode == ConstantRatio {
		fmt.Println("Staggered for:", r.StaggeredFor)
	}
	fmt.Println("Average OP/s: ", r.AverageOps)
	fmt.Println("Average good OP/s: ", r.AverageGoodOps)
	fmt.Println()
}

func (o *HumanOutput) printResults(r *Results) {
	o.printOpResult(&r.Writes, "Writes")
	o.printOpResult(&r.Reads, "Reads")
}

func (o *JSONOutput) progress(s string) {
}
func (o *JSONOutput) printResults(r *Results) {
	b, err := json.Marshal(r)
	if err == nil {
		fmt.Println(string(b))
	} else {
		fmt.Println(err.Error())
	}
}

func (o *HumanOutput) stopStream() {
	o.quit <- true

}

func (o *HumanOutput) error(s string) {
	o.stopStream()
	fmt.Println("Error: ", s)
}

func (o *HumanOutput) report(s string) {
	o.stopStream()
	fmt.Println(s)
}

func (o *JSONOutput) report(s string) {
}

func (o *JSONOutput) error(s string) {
}
