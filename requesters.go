package main

import (
	randc "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"io/ioutil"

	"github.com/tigrawap/goader/utils"
	"github.com/valyala/fasthttp"
	"io"
	"log"
	"os"
	"path"
)

type nullRequester struct {
}

type sleepRequster struct {
	state *OPState
	db    chan int
}

func (n *nullRequester) request(responses chan *Response, request *Request) {
	responses <- &Response{request, time.Nanosecond, nil}
}

func (requester *sleepRequster) request(responses chan *Response, request *Request) {
	if rand.Intn(10000)-int(requester.state.inFlight) < 0 {
		responses <- &Response{request, 0, errors.New("Bad response")}
		return
	}
	start := time.Now()
	requester.db <- 0
	var timeToSleep = time.Duration(rand.Intn(200)) * time.Millisecond
	time.Sleep(timeToSleep)
	<-requester.db
	responses <- &Response{request, time.Since(start), nil}
}

func newSleepRequster(state *OPState) *sleepRequster {
	r := sleepRequster{
		state: state,
		db:    make(chan int, 10),
	}
	return &r
}

type httpRequester struct {
	data    []byte
	client  fasthttp.Client
	timeout time.Duration
	method  string
	auther  HTTPAuther
	state   *OPState
}

func newHTTPRequester(state *OPState, auther HTTPAuther) *httpRequester {
	requester := httpRequester{
		state:  state,
		auther: auther,
	}
	requester.state = state
	if state.op == WRITE {
		requester.method = "PUT"
		var maxSize uint64
		if config.maxBodySize != 0 {
			maxSize = config.maxBodySize
		} else {
			maxSize = config.bodySize
		}
		requester.data = make([]byte, maxSize)
		randc.Read(requester.data)
	} else {
		requester.method = "GET"
	}
	if config.maxLatency == NotSet {
		requester.timeout = 60 * time.Second
	} else {
		requester.timeout = 5 * config.maxLatency
	}
	return &requester
}

func getPayloadRandom(fullData []byte) []byte {
	if config.maxBodySize != 0 {
		randomSize := rand.Int63n(int64(config.maxBodySize - config.minBodySize))
		return fullData[0: int64(config.minBodySize)+randomSize]
	} else {
		return fullData
	}
}

func newEvenPayload() payloadGetter {
	if config.maxBodySize == 0 {
		return func(fullData []byte) []byte {
			return fullData
		}
	}

	roller := utils.NewWeightedRoller(int64(config.minBodySize), int64(config.maxBodySize), config.randomFairBuckets)
	f := func(fullData []byte) []byte {
		return fullData[0:roller.Roll()]
	}
	return f
}

type payloadGetter func(fullData []byte) []byte

var getPayload = getPayloadRandom

func (requester *httpRequester) request(responses chan *Response, request *Request) {
	var req *fasthttp.Request
	defer func() {
		if err := recover(); err != nil { //catch
			responses <- &Response{&Request{targeter:&BadUrlTarget{}, startTime:time.Now()}, time.Nanosecond,
				fmt.Errorf("Error: %s", "panic")}
			return
		}
	}()
	for i := 0; i < 10 && req == nil; i++ {
		req = fasthttp.AcquireRequest()
		if req == nil {
			log.Println("Could not acquire request object from fasthttp, retrying")
		}
	}
	if req == nil {
		log.Fatalln("Could not acquire request object from fasthttp pool after 10 retries")
	}
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI(request.getUrl())

	req.Header.SetMethodBytes([]byte(requester.method))
	req.Header.Set("Connection", "keep-alive")
	if requester.data != nil {
		req.SetBody(getPayload(requester.data))
	}
	requester.auther.sign(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	start := time.Now()
	err := requester.client.DoTimeout(req, resp, requester.timeout)
	timeSpent := time.Since(start)
	statusCode := resp.StatusCode()

	if err != nil {
		responses <- &Response{request, timeSpent,
			fmt.Errorf("Bad request: %s\n%s\n%s", err, resp.Header.String(), resp.Body())}
		return
	}

	switch statusCode {
	case fasthttp.StatusOK, fasthttp.StatusCreated:
		responses <- &Response{request, timeSpent, nil}
	default:
		responses <- &Response{request, timeSpent,
			fmt.Errorf("Error: %d \n%s \n%s ", resp.StatusCode(), resp.Header.String(), resp.Body())}
	}
}

type diskRequester struct {
	state *OPState
	data  []byte
}

func newDiskRequester(state *OPState) *diskRequester {
	r := diskRequester{
		state: state,
	}
	if state.op == WRITE {
		r.data = make([]byte, config.bodySize)
		randc.Read(r.data)
	}
	return &r
}

func (requester *diskRequester) request(responses chan *Response, request *Request) {
	requester.doRequest(responses, request, false)
}

func (requester *diskRequester) doRequest(responses chan *Response, request *Request, isRetry bool) {
	filename := request.getUrl()

	var err error
	start := time.Now()
	if requester.state.op == WRITE {
		err = ioutil.WriteFile(filename, getPayload(requester.data), 0644)
		if os.IsNotExist(err) && config.mkdirs {
			err = os.MkdirAll(path.Dir(filename), 0755)
			if !isRetry {
				requester.doRequest(responses, request, true)
				return
			}
		}
	} else {
		_, err = ioutil.ReadFile(filename)
	}
	responses <- &Response{request, time.Since(start), err}
}

type metaOpRequst string

const (
	opUnlink   metaOpRequst = "unlink"
	opTruncate              = "truncate"
	opMknod                 = "mknod"
	opWrite                 = "write"
	opRead                  = "read"
	opStat                  = "stat"
	opSetattr               = "setattr"
	opSymlink               = "symlink"
	opHardLink              = "hardlink"
	opRename              = "rename"
)

var allMetaOps = []metaOpRequst{
	opUnlink, opTruncate, opMknod, opWrite, opRead, opStat, opSetattr, opSymlink, opHardLink, opRename,
}

type metaRequester struct {
	state *OPState
	data  []byte
	ops   []metaOpRequst
	opLen int //pre-calculated ops len
}

func (r *metaRequester) request(responses chan *Response, request *Request) {
	r.doRequest(responses, request, false)
}


func (r *metaRequester) doRequest(responses chan *Response, request *Request, isRetry bool) {
	filename := request.getUrl()
	var err error
	start := time.Now()
	op := r.ops[rand.Intn(r.opLen)]
	switch op {
	case opWrite, opRead:
		var fd *os.File
		var flags int
		if op == opWrite {
			flags = os.O_WRONLY | os.O_CREATE
		}else{
			flags = os.O_RDONLY
		}
		if fd, err = os.OpenFile(filename, flags, 0644); err == nil {
			fd.Seek(rand.Int63n(int64(config.fileOffsetLimit)), io.SeekStart)
			if op == opWrite {
				fd.Write(getPayload(r.data))
			} else {
				fd.Read(make([]byte, len(getPayload(r.data)))) // TODO: Refactor random poller to stand-alone entity
			}
			fd.Close() // TODO: Defer and reuse FD
		}
	case opSetattr:
		err = os.Chown(filename, rand.Int(), rand.Int())
	case opTruncate:
		err = os.Truncate(filename, rand.Int63n(int64(config.fileOffsetLimit)))
	case opUnlink:
		err = os.Remove(filename)
	case opStat:
		_, err = os.Stat(filename)
	case opMknod:
		err = mknod(filename)
	case opSymlink:
		os.Remove(request.getUrl())
		err = os.Symlink(utils.GetAbsolute(request.targeter.get()), request.getUrl())
	case opHardLink:
		os.Remove(request.getUrl())
		err = os.Link(utils.GetAbsolute(request.targeter.get()), request.getUrl())
	case opRename:
		err = os.Rename(request.getUrl(), utils.GetAbsolute(request.targeter.get()))
	default:
		log.Panicln("Unknown IO")
	}
	if os.IsNotExist(err) && config.mkdirs {
		err = os.MkdirAll(path.Dir(filename), 0755)
		if !isRetry {
			r.doRequest(responses, request, true)
			return
		}
	}
	responses <- &Response{request, time.Since(start), err}
}

func newMetaRequester(state *OPState) *metaRequester {
	r := metaRequester{
		state: state,
	}
	if state.op == WRITE {
		r.data = make([]byte, config.bodySize)
		randc.Read(r.data)
	}

	for _, op := range config.metaOps {
		metaOp := metaOpRequst(op.op)
		switch metaOp {
		case opSetattr, opStat, opWrite, opUnlink, opTruncate, opMknod, opRead, opSymlink, opHardLink, opRename:
			for i := 0; i < op.weight; i++ {
				r.ops = append(r.ops, metaOp)
			}
		default:
			log.Panicln("Unknown meta op ", op.op)
		}
	}
	r.opLen = len(r.ops)
	return &r
}
