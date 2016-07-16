package main

import (
	randc "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"io/ioutil"

	"github.com/valyala/fasthttp"
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
	if rand.Intn(10000)-requester.state.inFlight < 0 {
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
		requester.data = make([]byte, config.bodySize)
		randc.Read(requester.data)
	} else {
		requester.method = "GET"
	}
	return &requester
}

func (requester *httpRequester) request(responses chan *Response, request *Request) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI(request.url)

	req.Header.SetMethodBytes([]byte(requester.method))
	req.Header.Set("Connection", "keep-alive")
	if requester.data != nil {
		req.SetBody(requester.data)
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
			fmt.Errorf("Bad request: %v", err)}
		return
	}

	switch statusCode {
	case fasthttp.StatusOK, fasthttp.StatusCreated:
		responses <- &Response{request, timeSpent, nil}
	default:
		responses <- &Response{request, timeSpent, fmt.Errorf("Error: %v ", resp.StatusCode())}
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
	filename := request.url

	var err error
	start := time.Now()
	if requester.state.op == WRITE {
		err = ioutil.WriteFile(filename, requester.data, 0644)
	} else {
		_, err = ioutil.ReadFile(filename)
	}
	responses <- &Response{request, time.Since(start), err}
}
