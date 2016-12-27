package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"strconv"

	"runtime"

	"bytes"

	"sync"

	randc "crypto/rand"
	"math/big"
	"math/rand"

	"github.com/dustin/go-humanize"
	"github.com/mgutz/ansi"
)

//Request struct
type Request struct {
	url       string
	startTime time.Time
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
	Http            = "http"
	S3              = "s3"
	Disk            = "disk"
	Null            = "null"
	NotSet          = -1
	NotSetString    = ""
	NotSetFloat64   = -1.0
	FormatHuman     = "human"
	FormatJSON      = "json"
)

var config struct {
	url                string //url/pattern
	rps                int
	wps                int
	rpw                float64
	writeThreads       int
	readThreads        int
	maxChannels        int
	verbose            bool
	maxRequests        int64
	bodySize           uint64
	minBodySize        uint64
	maxBodySize        uint64
	bodySizeInput      string
	minBodySizeInput   string
	maxBodySizeInput   string
	mode               string
	engine             string
	outputFormat       string
	showProgress       bool
	stopOnBadRate      bool
	output             Output
	syncSleep          time.Duration
	maxLatency         time.Duration
	S3ApiKey           string
	S3Bucket           string
	S3Endpoint         string
	S3Region           string
	S3SecretKey        string
	S3SignatureVersion int
	timelineFile       string
}

//OPResults result of specific operation, lately can be printed by different outputters
type OPResults struct {
	Errors         int64
	Done           int64
	AverageOps     int64
	AverageGoodOps int64
	FinalSpeed     int
	AverageSpeed   time.Duration
	TopTen         []time.Duration
	Percentiles    map[string]time.Duration
	StaggeredFor   time.Duration
}

//Results of benchmark execution, represents final output, marshalled directly into json in json output mode
type Results struct {
	Writes    OPResults
	Reads     OPResults
	Errors    []string
	Reports   []string
	StartTime time.Time
}

func startWorker(state *OPState, targeter Target, requester Requester, quit chan bool) {
	for {
		select {
		case <-quit:
			return
		case <-state.requests:
			requester.request(state.responses, &Request{targeter.get(), time.Now()})
		}
	}
}

type op int

//Operation type
const (
	READ  op = iota
	WRITE
)

//OPState contains state for OP(READ/WRITE)
type OPState struct {
	color            string
	speed            int
	done             int64
	errors           int64
	inFlight         int
	goodUrls         []string
	op               op
	name             string
	latencies        timeArray
	totalTime        time.Duration
	colored          func(string) string
	responses        chan *Response
	requests         chan int
	progress         chan bool
	inFlightUpdate   chan int
	inFlightCallback chan int
	timeline         TimeLine
}

func newOPState(op op, color string) *OPState {
	var name string
	if op == WRITE {
		name = "write"
	} else {
		name = "read"
	}
	state := OPState{
		op:             op,
		name:           name,
		color:          color,
		goodUrls:       make([]string, 0, config.maxRequests),
		colored:        ansi.ColorFunc(fmt.Sprintf("%s+h:black", color)),
		responses:      make(chan *Response, 1000),
		requests:       make(chan int, 1000),
		progress:       make(chan bool, 1000),
		inFlightUpdate: make(chan int, 1000),
	}
	go state.inFlightUpdater()
	return &state
}

// Progress of benchmark
type Progress struct {
	reads  *OPState
	writes *OPState
}

func (state *OPState) inFlightUpdater() {
	var update int
	for {
		update = <-state.inFlightUpdate
		state.inFlight += update
		if state.inFlightCallback != nil && update < 0 {
			select {
			case state.inFlightCallback <- state.inFlight:
				continue
			default:
				continue
			}
		}
	}
}

func processResponses(state *OPState, results *Results, adjuster Adjuster, w *sync.WaitGroup, quit chan bool) {
	var response *Response
	defer w.Done()
	for {
		select {
		case <-quit:
			return
		case response = <-state.responses:
			state.inFlightUpdate <- -1
			state.done++
			state.progress <- response.err == nil
			if response.err == nil {
				state.totalTime += response.latency
				state.goodUrls = append(state.goodUrls, response.request.url)
				state.latencies = append(state.latencies, response.latency)
			} else {
				if config.verbose {
					results.reportError(response.err.Error())
				}
				state.errors++
			}
			state.timeline = append(state.timeline, RequestTimes{
				Start:   response.request.startTime,
				Latency: response.latency,
				Success: response.err == nil,
			})
			adjuster.adjust(response)
		}
	}
}

type timeArray []time.Duration

func (a timeArray) Len() int {
	return len(a)
}
func (a timeArray) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a timeArray) Less(i, j int) bool {
	return a[i] < a[j]
}

func percentile(numbers timeArray, n int) time.Duration {
	i := len(numbers) * n / 100
	if i < 0 {
		return time.Duration(0 * time.Millisecond)
	}
	return numbers[i]
}

func fillResults(results *OPResults, state *OPState, startTime time.Time) {
	results.Percentiles = make(map[string]time.Duration)
	succesful := len(state.goodUrls)
	if succesful > 0 {
		results.AverageSpeed = state.totalTime / time.Duration(succesful)
		sort.Sort(state.latencies)
		percentiles := []int{99, 95, 90, 80, 70, 60, 50, 40, 30}

		for i := range percentiles {
			s := strconv.Itoa(percentiles[i])
			results.Percentiles[s] = percentile(state.latencies, percentiles[i])
		}
		if config.mode == LowLatency {
			results.FinalSpeed = state.speed
		}

		from := succesful - 10
		if from < 0 {
			from = 0
		}
		results.TopTen = state.latencies[from:]
	}

	results.Done = state.done
	results.Errors = state.errors
	results.AverageOps = int64(float64(state.done * int64(time.Second)) / float64(time.Since(startTime).Nanoseconds())) + 1
	results.AverageGoodOps = int64(float64((state.done - state.errors) * int64(time.Second)) / float64(time.Since(startTime).Nanoseconds())) + 1
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
	case LowLatency, ConstantThreads:
		operators.writeEmitter = &threadedEmitter{}
		if config.mode == LowLatency {
			operators.writeAdjuster = newLatencyAdjuster(progress.writes)
		} else {
			operators.writeAdjuster = &nullAdjuster{}
		}

		if config.rpw != NotSetFloat64 {
			operators.readEmitter = &boundEmitter{
				boundTo: &progress.writes.done,
				boundBy: &config.rpw,
			}
			operators.readAdjuster = &nullAdjuster{}
		} else {
			operators.readEmitter = &threadedEmitter{}
			if config.mode == LowLatency {
				operators.readAdjuster = newLatencyAdjuster(progress.reads)
			} else {
				operators.readAdjuster = &nullAdjuster{}
			}
		}
	case ConstantRatio:
		operators.readEmitter = newRateEmitter(config.rps)
		operators.writeEmitter = newRateEmitter(config.wps)
		operators.writeAdjuster = &nullAdjuster{}
		operators.readAdjuster = &nullAdjuster{}
	}
	switch config.engine {
	case Sleep:
		operators.writeRequster = newSleepRequster(progress.writes)
		operators.readRequester = newSleepRequster(progress.reads)
	case Upload, Http:
		operators.writeRequster = newHTTPRequester(progress.writes, &nullAuther{})
		operators.readRequester = newHTTPRequester(progress.reads, &nullAuther{})
	case S3:
		s3params := s3Params{
			secretKey: config.S3SecretKey,
			apiKey:    config.S3ApiKey,
			bucket:    config.S3Bucket,
			endpoint:  config.S3Endpoint,
		}
		var s3Auther S3Auther
		if config.S3SignatureVersion == 4 {
			s3Auther = &s3AutherV4{s3params}
		} else {
			s3Auther = &s3AutherV2{s3params}
		}
		operators.writeRequster = newHTTPRequester(progress.writes, s3Auther)
		operators.readRequester = newHTTPRequester(progress.reads, s3Auther)
	case Disk:
		operators.writeRequster = newDiskRequester(progress.writes)
		operators.readRequester = newDiskRequester(progress.reads)
	case Null:
		operators.writeRequster = &nullRequester{}
		operators.readRequester = &nullRequester{}
	default:
		panic("Unknown engine")
	}
	operators.writeTarget = newTemplatedTarget()
	if config.wps > 0 || config.writeThreads > 0 {
		operators.readTarget = &BoundTarget{
			bound: &progress.writes.goodUrls,
		}
	} else {
		operators.readTarget = newTemplatedTarget()
	}
	return operators
}

func quitOnInterrupt() chan bool {
	c := make(chan os.Signal)
	quit := make(chan bool)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	signal.Notify(c, syscall.SIGQUIT)
	signal.Notify(c, syscall.SIGABRT)
	signal.Notify(c, syscall.SIGINT)
	go func() {
		<-c
		quit <- true
	}()
	return quit
}

func p(s string) {
	config.output.progress(s)
}

//TODO: Don't really like this global variables, methods on result, global config. Need to rethink
func (r *Results) reportError(s string) {
	r.Errors = append(r.Errors, s)
	config.output.reportError(s)
}

func (r *Results) report(s string) {
	r.Reports = append(r.Reports, s)
	config.output.report(s)
}

func displayProgress(state *OPState) {
	rate := time.Second / 1000
	throttle := time.Tick(rate)
	output := make(map[bool]string)
	output[true] = state.colored(".")
	output[false] = state.colored("E")
	var buffer bytes.Buffer

	for {
		success := <-state.progress
		if config.engine == Null || !config.showProgress {
			continue
		}
		buffer.WriteString(output[success])
		select {
		case <-throttle:
			p(buffer.String())
			buffer.Truncate(0)
		default:
			continue
		}
	}
}

func badRateAborter(state *OPState, result *OPResults, rate *int, stop chan bool) {
	checkEvery := time.Millisecond * 3
	lastCheck := time.Now()
	for {
		if state.inFlight > *rate {
			result.StaggeredFor += time.Since(lastCheck)
		}
		if state.inFlight > *rate*2 && config.stopOnBadRate {
			stop <- true
		}
		lastCheck = time.Now()
		time.Sleep(checkEvery)
	}
}

func makeLoad() {
	interrupted := quitOnInterrupt()
	stopWorkers := make(chan bool)
	progress := Progress{
		writes: newOPState(WRITE, "blue"),
		reads:  newOPState(READ, "green"),
	}
	progress.reads.speed = config.readThreads
	progress.writes.speed = config.writeThreads
	operators := getOperators(&progress)

	go displayProgress(progress.writes)
	go displayProgress(progress.reads)
	results := Results{
		StartTime: time.Now(),
	}
	mainDone := make(chan bool)
	workersDone := make(chan bool)
	stoppedByRate := make(chan bool)
	w := &sync.WaitGroup{}
	w.Add(2)
	go func() {
	FOR_LOOP:
		for {
			select {
			case <-interrupted:
				results.report("Stopped by user")
				break FOR_LOOP
			case <-stoppedByRate:
				results.reportError("Could not sustain given rate")
				break FOR_LOOP
			default:
				time.Sleep(time.Millisecond)
			}
			if progress.reads.done+progress.writes.done >= config.maxRequests {
				results.report("Maximum requests count")
				break
			}
		}
		close(stopWorkers)
		mainDone <- true
		w.Wait()
		workersDone <- true
	}()

	go operators.writeEmitter.emitRequests(progress.writes)
	go operators.readEmitter.emitRequests(progress.reads)
	for i := 0; i < config.maxChannels; i++ {
		go startWorker(progress.reads, operators.readTarget, operators.readRequester, stopWorkers)
		go startWorker(progress.writes, operators.writeTarget, operators.writeRequster, stopWorkers)
	}
	go processResponses(progress.writes, &results, operators.writeAdjuster, w, stopWorkers)
	go processResponses(progress.reads, &results, operators.readAdjuster, w, stopWorkers)
	if config.mode == ConstantRatio {
		go badRateAborter(progress.writes, &results.Writes, &config.wps, stoppedByRate)
		go badRateAborter(progress.reads, &results.Reads, &config.rps, stoppedByRate)
	}

	<-mainDone
	select {
	case <-workersDone:
	case <-time.After(5 * time.Second):
		config.output.report("Workers close timed out")
	}
	fillResults(&results.Writes, progress.writes, results.StartTime)
	fillResults(&results.Reads, progress.reads, results.StartTime)
	buildTimeline(progress.writes, progress.reads)
	config.output.printResults(&results)
}

func validateParams() {
	if (config.wps != NotSet || config.rps != NotSet) && (config.writeThreads != NotSet || config.readThreads != NotSet) {
		println("OP/s and Threads flags are exclusive")
		os.Exit(3)
	}
	if config.writeThreads > config.maxChannels {
		config.maxChannels = config.writeThreads
	}
	if config.readThreads > config.maxChannels {
		config.maxChannels = config.readThreads
	}
	switch config.S3SignatureVersion {
	case 2, 4:
	default:
		println("Wrong signature version, only 2 and 4 supported")
		os.Exit(3)
	}
	if config.engine == S3 && config.S3SignatureVersion == 4 {
		if config.S3Region == NotSetString {
			println("Must specify region for request with V4 signature")
			os.Exit(3)
		}
	}
	if config.engine == S3 && config.S3Endpoint == NotSetString {
		println("Must specify s3 endpoint in s3 mode")
		os.Exit(3)
	}
	isSetMin := config.minBodySize != 0
	isSetMax := config.maxBodySize != 0
	if isSetMin || isSetMax {
		if !(isSetMin && isSetMax) {
			println("Should set both min/max body size")
			os.Exit(3)
		} else {
			if config.minBodySize > config.maxBodySize {
				println("Min body size should be less then max body size")
				os.Exit(3)
			}
		}
	}
}

func selectMode() {
	if config.maxLatency != NotSet {
		config.mode = LowLatency
	} else if config.writeThreads > 0 || config.readThreads > 0 {
		config.mode = ConstantThreads
	} else if config.wps > 0 || config.rps > 0 {
		config.mode = ConstantRatio
	} else {
		fmt.Println("Should specify one of rps/wps/rt/wt/max-latency")
		os.Exit(2)
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

func selectPrinter() {
	switch config.outputFormat {
	case FormatHuman:
		config.output = newHumanOutput()
	case FormatJSON:
		config.output = &JSONOutput{}
	default:
		fmt.Println("Unknown output format")
		os.Exit(2)
	}
}

func configure() {
	flag.IntVar(&config.rps, "rps", NotSet, "Reads per second")
	flag.IntVar(&config.wps, "wps", NotSet, "Writes per second")
	flag.IntVar(&config.writeThreads, "wt", NotSet, "Write threads")
	flag.IntVar(&config.readThreads, "rt", NotSet, "Read threads")
	flag.Float64Var(&config.rpw, "rpw", NotSetFloat64, "Reads per write")
	flag.Int64Var(&config.maxRequests, "max-requests", 10000, "Maximum requests before stopping")
	flag.IntVar(&config.maxChannels, "max-channels", 32, "Maximum threads")
	flag.StringVar(&config.bodySizeInput, "body-size", "160KiB", "Body size for put requests, in bytes.")
	flag.StringVar(&config.maxBodySizeInput, "max-body-size", NotSetString, "Maximum body size for put requests (will randomize)")
	flag.StringVar(&config.minBodySizeInput, "min-body-size", NotSetString, "Minimal body size for put requests (will randomize)")
	flag.StringVar(&config.engine, "requests-engine", Upload, "s3/sleep/upload/http")
	flag.DurationVar(&config.maxLatency, "max-latency", NotSet,
		"Max latency to allow when searching for maximum thread count")
	// flag.StringVar(&config.mode, "mode", LowLatency, "Testing mode [low-latency / constant]")
	flag.StringVar(&config.url, "url", NotSetString, "Url to submit requests")
	flag.StringVar(&config.S3ApiKey, "s3-api-key", NotSetString, "S3 api key")
	flag.StringVar(&config.S3SecretKey, "s3-secret-key", NotSetString, "S3 secret key")
	flag.StringVar(&config.S3Bucket, "s3-bucket", NotSetString, "S3 bucket")
	flag.StringVar(&config.S3Endpoint, "s3-endpoint", NotSetString, "S3 endpoint")
	flag.StringVar(&config.S3Region, "s3-region", NotSetString, "S3 region, must be set for V4 signature")
	flag.IntVar(&config.S3SignatureVersion, "s3-sign-ver", 4, "S3 signature version, defaults to 4, supports 2/4")
	flag.StringVar(&config.outputFormat, "output", FormatHuman, "Output format(human/json), defaults to human")
	flag.BoolVar(&config.showProgress, "show-progress", true, "Displays progress as dots")
	flag.BoolVar(&config.stopOnBadRate, "stop-on-bad-rate", false, "Stops benchmark if cant maintain rate")
	flag.BoolVar(&config.verbose, "verbose", false, "Verbose output on errors")
	flag.StringVar(&config.timelineFile, "timeline-file", NotSetString, "Path to timeline.html (visual representation of progress)")
	flag.Parse()
	var err error
	config.bodySize, err = humanize.ParseBytes(config.bodySizeInput)
	if config.minBodySizeInput != NotSetString {
		config.minBodySize, err = humanize.ParseBytes(config.minBodySizeInput)
	}
	if config.maxBodySizeInput != NotSetString {
		config.maxBodySize, err = humanize.ParseBytes(config.maxBodySizeInput)
		config.bodySize = config.maxBodySize
	}
	if err != nil {
		fmt.Println(err.Error())
	}
	validateParams()
	selectMode()
	selectPrinter()
	configureMode()
	n, err := randc.Int(randc.Reader, big.NewInt(1))
	rand.Seed(n.Int64())

}

func main() {
	configure()
	maxprocs := runtime.GOMAXPROCS(0)
	if maxprocs < 16 {
		runtime.GOMAXPROCS(16)
	}
	makeLoad()
}
