package main

import (
	"flag"
	"fmt"
	"sort"
	"time"
)

//Request struct
type Request struct {
	num int64
}

// Response of worker
type Response struct {
	request *Request
	latency time.Duration
	err     error
}

//Various constants to avoid typos
const (
	LowLatency      = "low-latency"
	ConstantRatio   = "constant"
	ConstantThreads = "constant-threads"
	Sleep           = "sleep"
	Upload          = "upload"
)

var config struct {
	url          string //url/pattern
	rps          int
	wps          int
	writeThreads int
	readThreads  int
	rpw          int
	maxChannels  int
	maxRequests  int64
	mode         string
	engine       string
}

const badResponseTime = 1000

func startWorker(channels *Channels, progress *Progress, operators *Operators) {
	// TODO: Ditch progress from here, it's used only for fake worker...
	var request *Request
	for {
		select {
		case request = <-channels.read.requests:
			operators.readRequester.request(&channels.read, request)
		case request = <-channels.write.requests:
			operators.writeRequster.request(&channels.write, request)
		}
	}
}

//OPChannels hold channels for communication for specific operation
type OPChannels struct {
	responses chan *Response
	requests  chan *Request
}

//Channels for communication per op
type Channels struct {
	read  OPChannels
	write OPChannels
}

//OPState contains state for OP(READ/WRITE)
type OPState struct {
	speed     int
	done      int64
	errors    int64
	inFlight  int
	num       int64
	goodNums  []int64
	latencies timeArray
	totalTime time.Duration
}

// Progress of benchmark
type Progress struct {
	reads  OPState
	writes OPState
}

func processResponse(response *Response, state *OPState) {
	state.inFlight--
	state.done++
	if response.err == nil {
		state.totalTime += response.latency
		state.goodNums = append(state.goodNums, response.request.num)
		state.latencies = append(state.latencies, response.latency)
	} else {
		state.errors++
	}
}

type timeArray []time.Duration

func (a timeArray) Len() int           { return len(a) }
func (a timeArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a timeArray) Less(i, j int) bool { return a[i] < a[j] }

func percentile(numbers timeArray, n int) time.Duration {
	i := len(numbers) * n / 100
	if i < 0 {
		return time.Duration(0 * time.Millisecond)
	}
	return numbers[i]
}

func printResults(state *OPState) {
	succesful := len(state.goodNums)
	if succesful > 0 {
		fmt.Println("Average response time:", state.totalTime/time.Duration(succesful))
	}
	if succesful > 0 {
		sort.Sort(state.latencies)
		percentiles := []int{95, 90, 80, 70, 60, 50, 40, 30}

		for i := range percentiles {
			fmt.Println("Percentile ", percentiles[i], percentile(state.latencies, percentiles[i]))
		}
	}

	fmt.Println("Total requests:", state.done)
	fmt.Println("Total errors:", state.errors)
	fmt.Println()
	fmt.Println()
}

//Operators chosen by config
type Operators struct {
	readEmitter   Emitter
	writeEmitter  Emitter
	readAdjuster  Adjuster
	writeAdjuster Adjuster
	readRequester Requester
	writeRequster Requester
}

func getOperators(progress *Progress) *Operators {
	operators := new(Operators)
	switch config.mode {
	case LowLatency:
		operators.readEmitter = &threadedEmitter{}
		operators.writeEmitter = &threadedEmitter{}
		operators.readAdjuster = &speedAdjuster{}
		operators.writeAdjuster = &speedAdjuster{}
	case ConstantThreads:
		operators.readEmitter = &threadedEmitter{}
		operators.writeEmitter = &threadedEmitter{}
		operators.writeAdjuster = &nullAdjuster{}
		operators.readAdjuster = &nullAdjuster{}
	case ConstantRatio:
		operators.readEmitter = newRateEmitter(config.rps, &progress.writes.goodNums, true)
		operators.writeEmitter = newRateEmitter(config.wps, nil, false)
		operators.writeAdjuster = &nullAdjuster{}
		operators.readAdjuster = &nullAdjuster{}
	}
	switch config.engine {
	case Sleep:
		operators.writeRequster = &sleepRequster{&progress.writes}
		operators.readRequester = &sleepRequster{&progress.reads}
	case Upload:
		operators.writeRequster = newHTTPRequester("write")
		operators.readRequester = newHTTPRequester("read")
	}
	return operators
}

func makeLoad() {
	channels := Channels{
		read: OPChannels{
			responses: make(chan *Response, 100),
			requests:  make(chan *Request, 1000)},
		write: OPChannels{
			responses: make(chan *Response, 100),
			requests:  make(chan *Request, 1000)},
	}
	progress := Progress{
		writes: OPState{
			goodNums: make([]int64, 0, config.maxRequests),
		},
		reads: OPState{
			goodNums: make([]int64, 0, config.maxRequests),
		},
	}
	progress.reads.speed = config.readThreads
	progress.writes.speed = config.writeThreads
	operators := getOperators(&progress)

	for i := 0; i < config.maxChannels; i++ {
		go startWorker(&channels, &progress, operators)
	}
	for {
		operators.writeEmitter.emitRequests(&channels.write, &progress.writes)
		operators.readEmitter.emitRequests(&channels.read, &progress.reads)
		var response *Response
		select {
		case response = <-channels.write.responses:
			operators.writeAdjuster.adjust(response, &progress.writes)
			processResponse(response, &progress.writes)
			// Got one; nothing more to do.
		case response = <-channels.read.responses:
			operators.readAdjuster.adjust(response, &progress.reads)
			processResponse(response, &progress.reads)
		default:
			time.Sleep(1 * time.Millisecond)
		}
		if progress.reads.done+progress.writes.done > config.maxRequests {
			fmt.Println("Exceeded maximum requests count")
			break
		}
		// TODO: Smarter aborter
		if config.mode == ConstantRatio {
			if progress.writes.inFlight+progress.reads.inFlight > (config.wps+config.rps)*2 {
				panic("Could not sustain given rate")
			}
		}
	}

	fmt.Println("Reads:")
	printResults(&progress.reads)
	fmt.Println("Writes:")
	printResults(&progress.writes)
}

func init() {
	flag.IntVar(&config.rps, "rps", 0, "Reads per second")
	flag.IntVar(&config.wps, "wps", 0, "Writes per second")
	flag.IntVar(&config.writeThreads, "wt", 0, "Write threads")
	flag.IntVar(&config.readThreads, "rt", 0, "Read threads")
	flag.IntVar(&config.rpw, "rpw", 10, "Reads per write ratio")
	flag.Int64Var(&config.maxRequests, "max-requests", 10000, "Maximum requests before stopping")
	flag.IntVar(&config.maxChannels, "max-channels", 500, "Maximum threads")
	flag.StringVar(&config.engine, "requests-engine", Sleep, "s3/sleep/upload")
	flag.StringVar(&config.mode, "mode", LowLatency, "Testing mode [low-latency / constant]")
	flag.StringVar(&config.url, "url", "", "Url to submit requests")
	flag.Parse()
	if config.writeThreads != 0 || config.readThreads != 0 {
		config.mode = ConstantThreads
	} else if config.wps != 0 || config.rps != 0 {
		config.mode = ConstantRatio
	} else {
		config.writeThreads = 1
		config.readThreads = 1
		config.mode = LowLatency
	}
}

func main() {
	makeLoad()
}
