package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/mgutz/ansi"
)

//Request struct
type Request struct {
	num int64
}

//Response struct
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
	Null            = "null"
	NotSet          = -1
)

var config struct {
	url             string //url/pattern
	rps             int
	wps             int
	rpw             int
	writeThreads    int
	readThreads     int
	maxChannels     int
	maxRequests     int64
	bodySize        int
	mode            string
	engine          string
	syncSleep       time.Duration
	badResponseTime time.Duration
}

func startWorker(channels *Channels, operators *Operators) {
	for {
		select {
		case <-channels.read.requests:
			operators.readRequester.request(&channels.read, &Request{operators.readTarget.get()})
		case <-channels.write.requests:
			operators.writeRequster.request(&channels.write, &Request{operators.writeTarget.get()})
		}
	}
}

//OPChannels hold channels for communication for specific operation
type OPChannels struct {
	responses chan *Response
	requests  chan int
}

//Channels for communication per op
type Channels struct {
	read  OPChannels
	write OPChannels
}
type op int

const (
	READ op = iota
	WRITE
)

//OPState contains state for OP(READ/WRITE)
type OPState struct {
	speed     int
	done      int64
	errors    int64
	inFlight  int
	goodNums  []int64
	op        op
	latencies timeArray
	totalTime time.Duration
	colored   func(string) string
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

func printResults(state *OPState, startTime time.Time) {
	succesful := len(state.goodNums)
	if succesful > 0 {
		fmt.Println("Average response time:", state.totalTime/time.Duration(succesful))
		sort.Sort(state.latencies)
		percentiles := []int{95, 90, 80, 70, 60, 50, 40, 30}

		for i := range percentiles {
			fmt.Println("Percentile ", percentiles[i], percentile(state.latencies, percentiles[i]))
		}
		if config.mode == LowLatency {
			fmt.Printf("Threads with latency below %v: %v\n", config.badResponseTime, state.speed)
		}
	}

	fmt.Println("Total requests:", state.done)
	fmt.Println("Total errors:", state.errors)
	fmt.Println("Average OP/s: ", int64(float64(state.done*int64(time.Second))/float64(time.Since(startTime).Nanoseconds()))+1)
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
	readTarget    Target
	writeTarget   Target
}

func getOperators(progress *Progress) *Operators {
	operators := new(Operators)
	switch config.mode {
	case LowLatency:
		operators.readEmitter = &threadedEmitter{}
		operators.writeEmitter = &threadedEmitter{}
		if config.writeThreads != 0 {
			operators.readAdjuster = &boundAdjuster{
				boundTo: &progress.writes.speed,
				boundBy: &config.rpw,
				state:   &progress.reads,
			}
		} else {
			operators.readAdjuster = newLatencyAdjuster(&progress.reads)
		}
		operators.writeAdjuster = newLatencyAdjuster(&progress.writes)
	case ConstantThreads:
		operators.readEmitter = &threadedEmitter{}
		operators.writeEmitter = &threadedEmitter{}
		operators.writeAdjuster = &nullAdjuster{}
		operators.readAdjuster = &nullAdjuster{}
	case ConstantRatio:
		operators.readEmitter = newRateEmitter(config.rps)
		operators.writeEmitter = newRateEmitter(config.wps)
		operators.writeAdjuster = &nullAdjuster{}
		operators.readAdjuster = &nullAdjuster{}
	}
	switch config.engine {
	case Sleep:
		operators.writeRequster = newSleepRequster(&progress.writes)
		operators.readRequester = newSleepRequster(&progress.reads)
	case Upload:
		operators.writeRequster = newHTTPRequester(&progress.writes)
		operators.readRequester = newHTTPRequester(&progress.reads)
	case Null:
		operators.writeRequster = &nullRequester{}
		operators.readRequester = &nullRequester{}
	default:
		panic("Unknown engine")
	}
	operators.writeTarget = newIncrementalTarget()
	if config.wps > 0 || config.writeThreads > 0 {
		operators.readTarget = &BoundTarget{
			bound: &progress.writes.goodNums,
		}
	} else {
		operators.readTarget = newIncrementalTarget()
	}
	return operators
}

func quitOnInterrupt() chan bool {
	c := make(chan os.Signal)
	quit := make(chan bool)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		quit <- true
	}()
	return quit
}

var pb = make(chan string, 1000)

func printer(quit chan bool) {
	for {
		select {
		case <-quit:
			println()
			return
		case s := <-pb:
			print(s)
		}
	}
}

func p(s string) {
	pb <- s
}

func makeLoad() {
	quit := quitOnInterrupt()
	stopPrinter := make(chan bool)
	go printer(stopPrinter)
	channels := Channels{
		read: OPChannels{
			responses: make(chan *Response, 100),
			requests:  make(chan int, config.maxChannels*2)},
		write: OPChannels{
			responses: make(chan *Response, 100),
			requests:  make(chan int, config.maxChannels*2)},
	}
	progress := Progress{
		writes: OPState{
			op:       WRITE,
			goodNums: make([]int64, 0, config.maxRequests),
			colored:  ansi.ColorFunc("blue+h:black"),
		},
		reads: OPState{
			op:       READ,
			goodNums: make([]int64, 0, config.maxRequests),
			colored:  ansi.ColorFunc("green+h:black"),
		},
	}
	progress.reads.speed = config.readThreads
	progress.writes.speed = config.writeThreads
	operators := getOperators(&progress)

	for i := 0; i < config.maxChannels*2; i++ {
		go startWorker(&channels, operators)
	}
	startTime := time.Now()
MAIN_LOOP:
	for {
		operators.writeEmitter.emitRequests(&channels.write, &progress.writes)
		operators.readEmitter.emitRequests(&channels.read, &progress.reads)
		var response *Response
		select {
		case <-quit:
			stopPrinter <- true
			fmt.Println("\n Stopped by user")
			break MAIN_LOOP
		case response = <-channels.write.responses:
			operators.writeAdjuster.adjust(response)
			processResponse(response, &progress.writes)
		case response = <-channels.read.responses:
			operators.readAdjuster.adjust(response)
			processResponse(response, &progress.reads)
		default:
			time.Sleep(config.syncSleep)
		}
		if progress.reads.done+progress.writes.done > config.maxRequests {
			stopPrinter <- true
			fmt.Println("Exceeded maximum requests count")
			break
		}
		// TODO: Smarter aborter
		if config.mode == ConstantRatio {
			if progress.writes.inFlight+progress.reads.inFlight > (config.wps+config.rps)*2 {
				stopPrinter <- true
				fmt.Println("\n Could not sustain given rate")
				os.Exit(2)
			}
		}
	}

	println()
	fmt.Println("Reads:")
	printResults(&progress.reads, startTime)
	fmt.Println("Writes:")
	printResults(&progress.writes, startTime)
}

func validateParams() {
	if (config.wps != NotSet || config.rps != NotSet) && (config.writeThreads != NotSet || config.readThreads != NotSet) {
		fmt.Println("OP/s and Threads flags are exclusive")
	}
}

func selectMode() {
	if config.writeThreads > 0 || config.readThreads > 0 {
		config.mode = ConstantThreads
	} else if config.wps > 0 || config.rps > 0 {
		config.mode = ConstantRatio
	} else {
		config.mode = LowLatency
	}
}

func configureMode() {
	switch config.mode {
	case Null:
		config.syncSleep = time.Nanosecond
	case LowLatency:
		if config.writeThreads == NotSet {
			config.writeThreads = 1
		}
		if config.readThreads == NotSet {
			config.readThreads = 1
		}
	default:
		config.syncSleep = 1000 * time.Nanosecond
	}
}

func configure() {
	flag.IntVar(&config.rps, "rps", NotSet, "Reads per second")
	flag.IntVar(&config.wps, "wps", NotSet, "Writes per second")
	flag.IntVar(&config.writeThreads, "wt", NotSet, "Write threads")
	flag.IntVar(&config.readThreads, "rt", NotSet, "Read threads")
	flag.IntVar(&config.rpw, "rpw", 1, "Reads per write in search-for maximum mode")
	flag.Int64Var(&config.maxRequests, "max-requests", 10000, "Maximum requests before stopping")
	flag.IntVar(&config.maxChannels, "max-channels", 500, "Maximum threads")
	flag.IntVar(&config.bodySize, "body-size", 1024*160, "Body size for put requests, in bytes")
	flag.StringVar(&config.engine, "requests-engine", Upload, "s3/sleep/upload")
	flag.DurationVar(&config.badResponseTime, "max-latency", time.Duration(1000*time.Millisecond),
		"Max latency to allow when searching for maximum thread count")
	// flag.StringVar(&config.mode, "mode", LowLatency, "Testing mode [low-latency / constant]")
	flag.StringVar(&config.url, "url", "", "Url to submit requests")
	flag.Parse()
	validateParams()
	selectMode()
	configureMode()

}

func main() {
	configure()
	makeLoad()
}
