package main

import (
	randc "crypto/rand"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"time"

	"github.com/valyala/fasthttp"
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
	LowLatency    = "low-latency"
	ConstantRatio = "constant"
	Sleep         = "sleep"
	Upload        = "upload"
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

// Emitter of requests
type Emitter interface {
	emitRequests(channels *OPChannels, state *OPState)
}

type threadedEmitter struct{}

func (emitter *threadedEmitter) emitRequests(channels *OPChannels, state *OPState) {
	for state.speed > state.inFlight {
		state.num++
		channels.requests <- &Request{state.num}
		state.inFlight++
	}
}

func newRateEmitter(perSecond int, allowedNums *[]int64, useAllowed bool) *rateEmitter {
	var emitEvery time.Duration
	if perSecond != 0 {
		emitEvery = time.Duration(time.Second) / time.Duration(perSecond)
	} else {
		emitEvery = 0
	}
	emitter := new(rateEmitter)
	if allowedNums != nil {
		emitter.allowedNums = allowedNums
	}
	emitter.startedAt = time.Now()
	emitter.emitEvery = emitEvery
	emitter.useAllowed = useAllowed
	return emitter
}

type rateEmitter struct {
	startedAt    time.Time
	emitEvery    time.Duration
	totalEmitted int64
	allowedNums  *[]int64
	useAllowed   bool
}

func (emitter *rateEmitter) emitRequests(channels *OPChannels, state *OPState) {
	if emitter.emitEvery == 0 {
		return
	}

	totalTimePassed := time.Since(emitter.startedAt)
	shouldEmit := int64(totalTimePassed / emitter.emitEvery)
	notEmitted := shouldEmit - emitter.totalEmitted
	var i int64
	for i = 0; i < notEmitted; i++ {
		state.num++
		var num int64
		if emitter.useAllowed {
			nums := *emitter.allowedNums
			if len(nums) == 0 {
				break
			}
			num = nums[rand.Intn(int(len(nums)))]
		} else {
			num = state.num
		}
		state.inFlight++
		emitter.totalEmitted++
		channels.requests <- &Request{num}
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

//Adjuster should decide whether change throughput based on response
type Adjuster interface {
	adjust(response *Response, state *OPState)
}

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

//Requester does actual requests to server/fs
type Requester interface {
	request(channels *OPChannels, request *Request)
}

type sleepRequster struct {
	state *OPState
}

func (requester *sleepRequster) request(channels *OPChannels, request *Request) {
	if rand.Intn(10000)-requester.state.inFlight < 0 {
		fmt.Println("Error")
		channels.responses <- &Response{request, 0, errors.New("Bad response")}
		return
	}
	var timeToSleep = time.Duration(requester.state.inFlight*rand.Intn(10)) * time.Millisecond
	time.Sleep(timeToSleep)
	channels.responses <- &Response{request, timeToSleep, nil}
}

type httpRequester struct {
	data             []byte
	client           fasthttp.Client
	method           string
	pattern          *regexp.Regexp
	patternFormatter string
}

func newHTTPRequester(mode string) *httpRequester {
	if config.url == "" {
		panic("Must specify url for requests")
	}
	requester := new(httpRequester)
	requester.pattern = regexp.MustCompile(`(X{2,})`)
	length := len(requester.pattern.FindString(config.url))
	requester.patternFormatter = fmt.Sprintf("%%0%dd", length)
	if mode == "write" {
		requester.method = "PUT"
		requester.data = make([]byte, 160*1024, 160*1024)
	} else {
		requester.method = "GET"
	}
	randc.Read(requester.data)
	return requester
}

func (requester *httpRequester) request(channels *OPChannels, request *Request) {
	print(".")
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(requester.pattern.ReplaceAllString(config.url,
		fmt.Sprintf(requester.patternFormatter,
			request.num)))

	req.Header.SetMethodBytes([]byte(requester.method))
	req.Header.Set("Connection", "keep-alive")
	if requester.data != nil {
		req.SetBody(requester.data)
	}
	start := time.Now()
	resp := fasthttp.AcquireResponse()
	timeSpent := time.Since(start)
	err := requester.client.Do(req, resp)
	statusCode := resp.StatusCode()
	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

	if err != nil {
		channels.responses <- &Response{request, 0,
			fmt.Errorf("Bad request: %v", err)}
	}

	switch statusCode {
	case fasthttp.StatusOK, fasthttp.StatusCreated:
		channels.responses <- &Response{request, timeSpent, nil}
	default:
		channels.responses <- &Response{request, timeSpent, fmt.Errorf("Error: statusCode")}
	}
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
		config.mode = LowLatency
	} else if config.wps != 0 || config.rps != 0 {
		config.mode = ConstantRatio
	}
}

func main() {
	makeLoad()
}
