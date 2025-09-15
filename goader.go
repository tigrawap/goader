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

	"errors"
	"github.com/dustin/go-humanize"
	"github.com/mgutz/ansi"
	"log"
	"strings"
	"sync/atomic"
)

//Request struct
type Request struct {
	targeter  Target
	url       string
	startTime time.Time
}

func (r *Request) getUrl() string {
	if r.url == "" {
		r.url = r.targeter.get()
	}
	return r.url
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
	Meta            = "meta"
	Null            = "null"
	NotSet          = -1
	EmptyString     = ""
	NotSetString    = "_GOADER_NOT_SET_"
	NotSetFloat64   = -1.0
	FormatHuman     = "human"
	FormatJSON      = "json"
	HttpsScheme     = "https"
	HttpScheme      = "http"
)

var config struct {
	url                    string //url/pattern
	urlsSourceFile         string //url/pattern
	writtenUrlsDump        string
	rps                    int
	wps                    int
	rpw                    float64
	writeThreads           int
	readThreads            int
	mkdirs                 bool
	maxChannels            int
	verbose                bool
	memoryDebug            bool
	maxRequests            int64
	bodySize               uint64
	minBodySize            uint64
	maxBodySize            uint64
	bodySizeInput          string
	minBodySizeInput       string
	maxBodySizeInput       string
	metaOps                metaOps
	metaXattrKeys          int
	metaXattrLength        int
	randomFairDistribution bool
	randomFairBuckets      int
	fileOffsetLimitInput   string
	fileOffsetLimit        uint64
	mode                   string
	engine                 string
	outputFormat           string
	showProgress           bool
	stopOnBadRate          bool
	adjustOnErrors         bool
	output                 Output
	syncSleep              time.Duration
	maxLatency             time.Duration
	S3ApiKey               string
	S3Bucket               string
	S3Endpoint             string
	S3HttpScheme           string
	S3Region               string
	S3SecretKey            string
	S3SignatureVersion     int
	timelineFile           string
	seed                   int64
	writeGoodUrls          bool
	insecureHttps          bool
	chunkSizeInput         string
	chunkSize              uint64
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

func startWorker(progress *Progress, state *OPState, targeter Target, requester Requester, quit chan bool, w *sync.WaitGroup) {
	defer w.Done()
	for {
		select {
		case <-quit:
			return
		case <-state.requests:
			if !(atomic.AddInt64(&progress.totalRequests, 1) > config.maxRequests) {
				requester.request(state.responses, &Request{targeter: targeter, startTime: time.Now()})
			} else {
				return
			}
		}
	}
}

type op int

//Operation type
const (
	READ op = iota
	WRITE
)

//OPState contains state for OP(READ/WRITE)
type OPState struct {
	color            string
	concurrency      int
	done             int64
	errors           int64
	inFlight         int32
	slicesLock       sync.RWMutex
	goodUrls         []string
	op               op
	name             string
	latencies        timeArray
	totalTime        time.Duration
	colored          func(string) string
	responses        chan *Response
	requests         chan int
	progress         chan bool
	inFlightCallback chan int32
	timeline         TimeLine
}

func newOPState(op op, color string) *OPState {
	var name string
	if op == WRITE {
		name = "write"
	} else {
		name = "read"
	}
	buffersLen := 1000
	overrides := []int{config.maxChannels, config.writeThreads, config.readThreads}
	for _, o := range overrides {
		if o > buffersLen {
			buffersLen = o
		}
	}
	state := OPState{
		op:        op,
		name:      name,
		color:     color,
		latencies: make(timeArray, 0, config.maxRequests),
		colored:   ansi.ColorFunc(fmt.Sprintf("%s+h:black", color)),
		responses: make(chan *Response, buffersLen),
		requests:  make(chan int, buffersLen),
		progress:  make(chan bool, buffersLen),
	}
	if config.timelineFile != EmptyString {
		state.timeline = make([]RequestTimes, 0, config.maxRequests)
	}
	if op == WRITE && config.writeGoodUrls {
		state.goodUrls = make([]string, 0, config.maxRequests)
	}
	return &state
}

// Progress of benchmark
type Progress struct {
	totalRequests int64
	reads         *OPState
	writes        *OPState
}

func (state *OPState) opDone() {
	atomic.AddInt64(&state.done, 1)
	go state.inFlightDecrease()
}

func (state *OPState) getDone() int64 {
	return atomic.LoadInt64(&state.done)
}

func (state *OPState) inFlightDecrease() {
	res := atomic.AddInt32(&state.inFlight, -1)
	state.inFlightCallback <- res
}

func processResponses(state *OPState, results *Results, adjuster Adjuster, w *sync.WaitGroup, quit chan bool) {
	var response *Response
	defer w.Done()
	for {
		select {
		case <-quit:
			return
		case response = <-state.responses:
			state.opDone()
			state.progress <- response.err == nil
			if response.err == nil {
				state.totalTime += response.latency
				state.slicesLock.Lock()
				if state.op == WRITE && config.writeGoodUrls {
					state.goodUrls = append(state.goodUrls, response.request.getUrl())
				}
				state.latencies = append(state.latencies, response.latency)
				state.slicesLock.Unlock()
			} else {
				if config.verbose {
					results.reportError(response.err.Error())
				}
				state.errors++
			}
			if config.timelineFile != EmptyString {
				state.timeline = append(state.timeline, RequestTimes{
					Start:   response.request.startTime,
					Latency: response.latency,
					Success: response.err == nil,
				})
			}
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
	successful := len(state.latencies)
	if successful > 0 {
		results.AverageSpeed = state.totalTime / time.Duration(successful)
		sort.Sort(state.latencies)
		percentiles := []int{99, 95, 90, 80, 70, 60, 50, 40, 30}

		for i := range percentiles {
			s := strconv.Itoa(percentiles[i])
			results.Percentiles[s] = percentile(state.latencies, percentiles[i])
		}
		if config.mode == LowLatency {
			results.FinalSpeed = state.concurrency
		}

		from := successful - 10
		if from < 0 {
			from = 0
		}
		results.TopTen = state.latencies[from:]
	}

	results.Done = state.getDone()
	results.Errors = state.errors
	results.AverageOps = int64(float64(state.getDone()*int64(time.Second))/float64(time.Since(startTime).Nanoseconds())) + 1
	results.AverageGoodOps = int64(float64((state.getDone()-state.errors)*int64(time.Second))/float64(time.Since(startTime).Nanoseconds())) + 1
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
			secretKey:  config.S3SecretKey,
			apiKey:     config.S3ApiKey,
			bucket:     config.S3Bucket,
			endpoint:   config.S3Endpoint,
			httpScheme: config.S3HttpScheme,
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
	case Meta:
		operators.writeRequster = newMetaRequester(progress.writes)
		operators.readRequester = newMetaRequester(progress.reads)
	case Null:
		operators.writeRequster = &nullRequester{}
		operators.readRequester = &nullRequester{}
	default:
		log.Println("Unknown engine")
		os.Exit(1)
	}

	operators.writeTarget = selectTargetByConfig()
	if config.wps > 0 || config.writeThreads > 0 {
		operators.readTarget = &BoundTarget{
			bound: &progress.writes.goodUrls,
			sync:  progress.writes.slicesLock.RLocker(),
		}
	} else {
		operators.readTarget = selectTargetByConfig()
	}
	return operators
}

func quitOnInterrupt() chan bool {
	c := make(chan os.Signal)
	quit := make(chan bool)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGQUIT)
	signal.Notify(c, syscall.SIGABRT)
	go func() {
		<-c
		quit <- true
	}()
	return quit
}

func quitOnInterruptGracefully() chan bool {
	c := make(chan os.Signal)
	quit := make(chan bool)
	signal.Notify(c, syscall.SIGTERM)
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
		if state.inFlight > int32(*rate) {
			result.StaggeredFor += time.Since(lastCheck)
		}
		if state.inFlight > int32(*rate)*2 && config.stopOnBadRate {
			stop <- true
		}
		lastCheck = time.Now()
		time.Sleep(checkEvery)
	}
}

func makeLoad() {
	interrupted := quitOnInterrupt()
	gracefullyInterrupted := quitOnInterruptGracefully()
	stopWorkers := make(chan bool)
	progress := Progress{
		writes: newOPState(WRITE, "blue"),
		reads:  newOPState(READ, "green"),
	}
	progress.reads.concurrency = config.readThreads
	progress.writes.concurrency = config.writeThreads
	operators := getOperators(&progress)

	go displayProgress(progress.writes)
	go displayProgress(progress.reads)
	results := Results{
		StartTime: time.Now(),
	}
	mainDone := make(chan bool)
	workersDone := make(chan bool)
	stoppedByRate := make(chan bool)
	responseWait := &sync.WaitGroup{}
	responseWait.Add(2)

	workersWait := &sync.WaitGroup{}
	workersWait.Add(config.maxChannels * 2)

	forceExit := false
	go func() {
	FOR_LOOP:
		for {
			select {
			case <-interrupted:
				forceExit = true
				results.report("Aborted by user")
				break FOR_LOOP
			case <-gracefullyInterrupted:
				results.report("Stopped by user")
				break FOR_LOOP
			case <-stoppedByRate:
				results.reportError("Could not sustain given rate")
				break FOR_LOOP
			default:
				time.Sleep(time.Millisecond)
			}
			if progress.reads.getDone()+progress.writes.getDone() >= config.maxRequests {
				results.report("Maximum requests count")
				break FOR_LOOP
			}
		}
		close(stopWorkers)
		mainDone <- true
		if !forceExit {
			workersWait.Wait() // Letting IOs finish. In case of SIGABRT it will not wait.
		}
		responseWait.Wait()
		workersDone <- true
	}()

	go operators.writeEmitter.emitRequests(progress.writes)
	go operators.readEmitter.emitRequests(progress.reads)

	for i := 0; i < config.maxChannels; i++ {
		go startWorker(&progress, progress.reads, operators.readTarget, operators.readRequester, stopWorkers, workersWait)
		go startWorker(&progress, progress.writes, operators.writeTarget, operators.writeRequster, stopWorkers, workersWait)
	}
	go processResponses(progress.writes, &results, operators.writeAdjuster, responseWait, stopWorkers)
	go processResponses(progress.reads, &results, operators.readAdjuster, responseWait, stopWorkers)
	if config.mode == ConstantRatio {
		go badRateAborter(progress.writes, &results.Writes, &config.wps, stoppedByRate)
		go badRateAborter(progress.reads, &results.Reads, &config.rps, stoppedByRate)
	}

	//go func() {
	//	<-time.After(1 * time.Minute)
	//	buf := make([]byte, 1<<16)
	//	runtime.Stack(buf, true)
	//	fmt.Printf("%s", buf)
	//	os.Exit(1)
	//}()
	<-mainDone

	interrupted = quitOnInterrupt()
	gracefullyInterrupted = quitOnInterruptGracefully()
	select {
	case <-workersDone:
	case <-interrupted:
	case <-gracefullyInterrupted:
	case <-time.After(10 * time.Minute):
		config.output.report("Workers close timed out")
	}
	fillResults(&results.Writes, progress.writes, results.StartTime)
	fillResults(&results.Reads, progress.reads, results.StartTime)
	buildTimeline(progress.writes, progress.reads)
	dumpWrittenUrls(progress.writes.goodUrls) // TODO: Consider writing in real time? So it won't be dependant on goodUrls store, which can be quite huge
	config.output.printResults(&results)
}

func setParams() {
	var err error

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
	if config.engine == S3 && config.S3Endpoint == EmptyString {
		println("Must specify s3 endpoint in s3 mode")
		os.Exit(3)
	}

	if config.engine == S3 && strings.HasPrefix(config.S3Endpoint, "http://") {
		config.S3Endpoint = strings.Replace(config.S3Endpoint, "http://", "", 1)
	}
	if config.engine == S3 && strings.HasPrefix(config.S3Endpoint, "https://") {
		config.S3Endpoint = strings.Replace(config.S3Endpoint, "https://", "", 1)
		config.S3HttpScheme = HttpsScheme
	}
	if config.engine == S3 && config.S3HttpScheme == "" {
		config.S3HttpScheme = HttpScheme
	}

	//isSetMin := config.minBodySize != 0
	if config.minBodySizeInput != NotSetString {
		config.minBodySize, err = humanize.ParseBytes(config.minBodySizeInput)
	}
	if config.maxBodySizeInput != NotSetString {
		config.maxBodySize, err = humanize.ParseBytes(config.maxBodySizeInput)
	}
	if config.fileOffsetLimitInput != NotSetString {
		config.fileOffsetLimit, err = humanize.ParseBytes(config.fileOffsetLimitInput)
	}
	config.chunkSize, err = humanize.ParseBytes(config.chunkSizeInput)
	if err != nil {
		fmt.Println(err.Error())
	}

	isSetMax := config.maxBodySize != 0
	if isSetMax {
		if config.minBodySize > config.maxBodySize {
			println("Min body size should be less than max body size")
			os.Exit(3)
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
	case Meta:
		if len(config.metaOps) == 0 {
			for _, op := range allMetaOps {
				config.metaOps = append(config.metaOps, metaOp{string(op), 1})
			}
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

type metaOp struct {
	op     string
	weight int
}
type metaOps []metaOp

// String is the method to format the flag's value, part of the flag.Value interface.
// The String method's output will be used in diagnostics.
func (m *metaOps) String() string {
	return fmt.Sprint(*m)
}

func (m *metaOps) Set(value string) error {
	for _, weightedOp := range strings.Split(value, ",") {
		parts := strings.Split(weightedOp, ":")
		op := parts[0]
		weight := 1
		var err error
		switch len(parts) {
		case 0:
			log.Panicln()
			return errors.New(fmt.Sprintln("OP cannot be empty string"))
		case 1:
		case 2:
			weight, err = strconv.Atoi(parts[1])
			if err != nil {
				return errors.New(fmt.Sprintln("Could not parse weight of ", op))
			}
		default:
			log.Panicln("Could not parse meta ops, multiple ':' specified")
		}
		*m = append(*m, metaOp{op, weight})
	}
	return nil
}

func setRandomData() {
	dataSize := config.bodySize
	if config.maxBodySize != 0 {
		dataSize = config.maxBodySize
	}
	fullData := make([]byte, dataSize)
	randc.Read(fullData)
	requestersConfig.fullData = fullData
	requestersConfig.scratchData = make([]byte, dataSize)
}

func setPayloadGetter() {
	fullData := requestersConfig.fullData
	if config.maxBodySize == 0 {
		requestersConfig.payloadGetter = newFullPayload(fullData)
	} else {
		if config.randomFairDistribution {
			requestersConfig.payloadGetter = newFairPayload(fullData, int64(config.minBodySize), config.randomFairBuckets)
		} else {
			requestersConfig.payloadGetter = newRandomPayload(fullData, int64(config.minBodySize))
		}
	}
	requestersConfig.scratchBufferGetter = newScratchDataPayloadGetter(len(requestersConfig.fullData))
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func startMemoryPrint() {
	if !config.memoryDebug {
		return
	}
	for {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		// For info on each, see: https://golang.org/pkg/runtime/#MemStats
		var memoryInfo string
		memoryInfo += fmt.Sprintf("Alloc = %v MiB", bToMb(m.Alloc))
		memoryInfo += fmt.Sprintf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
		memoryInfo += fmt.Sprintf("\tSys = %v MiB", bToMb(m.Sys))
		memoryInfo += fmt.Sprintf("\tNumGC = %v\n", m.NumGC)
		log.Println("Memory info", memoryInfo)
		time.Sleep(5 * time.Second)
	}
}

func configure() {
	flag.IntVar(&config.rps, "rps", NotSet, "Reads per second")
	flag.IntVar(&config.wps, "wps", NotSet, "Writes per second")
	flag.IntVar(&config.writeThreads, "wt", NotSet, "Write threads")
	flag.IntVar(&config.readThreads, "rt", NotSet, "Read threads")
	flag.Float64Var(&config.rpw, "rpw", NotSetFloat64, "Reads per write")
	flag.Int64Var(&config.maxRequests, "max-requests", 10000, "Maximum requests before stopping")
	flag.IntVar(&config.randomFairBuckets, "random-fair-buckets", 1000, "Buckets for fair distribution, more buckets add precision, but increase cpu overhead.")
	flag.IntVar(&config.maxChannels, "max-channels", 32, "Maximum threads")
	flag.StringVar(&config.bodySizeInput, "body-size", "160KiB", "Body size for put requests, in bytes.")
	flag.StringVar(&config.maxBodySizeInput, "max-body-size", NotSetString, "Maximum body size for put requests (will randomize)")
	flag.StringVar(&config.minBodySizeInput, "min-body-size", NotSetString, "Minimal body size for put requests (will randomize)")
	flag.BoolVar(&config.randomFairDistribution, "fair-random", false, "Will produce fair distribution of body sizes, i.e with size 1-100 will produce ~100 requests of 1 byte and 1 requests of 100 bytes")
	flag.StringVar(&config.engine, "requests-engine", Disk, "s3/sleep/upload/http")
	flag.DurationVar(&config.maxLatency, "max-latency", NotSet,
		"Max latency to allow when searching for maximum thread count")
	// flag.StringVar(&config.mode, "mode", LowLatency, "Testing mode [low-latency / constant]")
	flag.StringVar(&config.url, "url", EmptyString, "Url to submit requests")
	flag.StringVar(&config.urlsSourceFile, "url-source", EmptyString, "File to read urls from, instead of --url template(built manually or generated by --written-urls-dump)")
	flag.StringVar(&config.S3ApiKey, "s3-api-key", EmptyString, "S3 api key")
	flag.StringVar(&config.S3SecretKey, "s3-secret-key", EmptyString, "S3 secret key")
	flag.StringVar(&config.S3Bucket, "s3-bucket", EmptyString, "S3 bucket")
	flag.StringVar(&config.S3Endpoint, "s3-endpoint", EmptyString, "S3 endpoint")
	flag.StringVar(&config.S3Region, "s3-region", NotSetString, "S3 region, must be set for V4 signature")
	flag.IntVar(&config.S3SignatureVersion, "s3-sign-ver", 4, "S3 signature version, defaults to 4, supports 2/4")
	flag.StringVar(&config.outputFormat, "output", FormatHuman, "Output format(human/json), defaults to human")
	flag.StringVar(&config.writtenUrlsDump, "written-urls-dump", EmptyString, "Path to dump written files, so then can rerun as reads job")
	flag.BoolVar(&config.showProgress, "show-progress", true, "Displays progress as dots")
	flag.BoolVar(&config.stopOnBadRate, "stop-on-bad-rate", false, "Stops benchmark if cant maintain rate")
	flag.BoolVar(&config.mkdirs, "mkdirs", false, "mkdir missing dirs on-write")
	flag.BoolVar(&config.verbose, "verbose", false, "Verbose output on errors")
	flag.BoolVar(&config.memoryDebug, "memory-debug", false, "Print out memory usage information to stderr every 10 seconds")
	flag.BoolVar(&config.adjustOnErrors, "adjust-on-errors", true, "Adjust concurrency in max-latency mode on errors")
	flag.StringVar(&config.timelineFile, "timeline-file", EmptyString, "Path to timeline.html (visual representation of progress)")
	flag.BoolVar(&config.insecureHttps, "insecure-https", false, "Disable SSL certificate validation")
	flag.Var(&config.metaOps, "meta-ops", "Comma-separated list of meta ops, can specify weight with :int, i.e write:3,unlink:1")
	flag.IntVar(&config.metaXattrKeys, "meta-xattr-keys", 10, "Maximum number of xattr values in file")
	flag.IntVar(&config.metaXattrLength, "meta-xattr-length", 5120, "Maximum length of xattrs, using weighed algorithm to distribute the sizes")
	flag.StringVar(&config.fileOffsetLimitInput, "meta-offset-limit", "16MiB", "Limit of offset for writes/truncate")
	flag.Int64Var(&config.seed, "seed", NotSet, "Seed to use in random generator")
	flag.StringVar(&config.chunkSizeInput, "chunk-size", "0", "Chunk size for disk reads. 0 means read whole file in one operation. Examples: 512KiB, 1MiB")
	flag.Parse()
	var err error
	config.bodySize, err = humanize.ParseBytes(config.bodySizeInput)
	if config.url == EmptyString && flag.NArg() == 1 {
		config.url = flag.Args()[0]
	}

	if err != nil {
		fmt.Println(err.Error())
	}
	setParams()
	setRandomData()
	setPayloadGetter()
	selectMode()
	selectPrinter()
	configureMode()
	configureGoodUrlsStore()
	n, err := randc.Int(randc.Reader, big.NewInt(1<<63-1))
	if config.seed == NotSet {
		rand.Seed(n.Int64())
	} else {
		rand.Seed(config.seed)
	}

}

func configureGoodUrlsStore() {
	if (config.writeThreads != 0 && config.readThreads != 0) || config.writtenUrlsDump != EmptyString {
		config.writeGoodUrls = true
	}
}

func main() {
	configure()
	maxprocs := runtime.GOMAXPROCS(0)
	if maxprocs < 16 {
		runtime.GOMAXPROCS(16)
	}
	go startMemoryPrint()
	makeLoad()
}
