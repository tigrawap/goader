package main

import (
	randc "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"io/ioutil"

	"github.com/valyala/fasthttp"
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
	fmt.Fprint(ioutil.Discard, request.url)
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
	data   []byte
	client fasthttp.Client
	method string
	auther HTTPAuther
	state  *OPState
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
	return &requester
}

func getPayload(fullData []byte) []byte {
	if config.maxBodySize != 0 {
		randomSize := rand.Int63n(int64(config.maxBodySize - config.minBodySize))
		return fullData[0 : int64(config.minBodySize)+randomSize]
	} else {
		return fullData
	}
}

func (requester *httpRequester) request(responses chan *Response, request *Request) {
	var req *fasthttp.Request
	defer func() {
		if err := recover(); err != nil { //catch
			responses <- &Response{&Request{"BAD_URL", time.Now()}, time.Nanosecond,
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
	req.SetRequestURI(request.url)

	req.Header.SetMethodBytes([]byte(requester.method))
	req.Header.Set("Connection", "keep-alive")
	if requester.data != nil {
		req.SetBody(getPayload(requester.data))
	}
	requester.auther.sign(req)
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)
	start := time.Now()
	err := requester.client.Do(req, resp)
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
	filename := request.url

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
