package main

import (
	randc "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"time"

	"github.com/valyala/fasthttp"
)

type nullRequester struct {
}

func (n *nullRequester) request(channels *OPChannels, request *Request) {
	channels.responses <- &Response{request, time.Nanosecond, nil}
}

type sleepRequster struct {
	state *OPState
	db    chan int
}

func (requester *sleepRequster) request(channels *OPChannels, request *Request) {
	if rand.Intn(10000)-requester.state.inFlight < 0 {
		p(requester.state.colored("E"))
		channels.responses <- &Response{request, 0, errors.New("Bad response")}
		return
	}
	p(requester.state.colored("."))
	start := time.Now()
	requester.db <- 0
	var timeToSleep = time.Duration(rand.Intn(200)) * time.Millisecond
	time.Sleep(timeToSleep)
	<-requester.db
	channels.responses <- &Response{request, time.Since(start), nil}
}

func newSleepRequster(state *OPState) *sleepRequster {
	r := sleepRequster{
		state: state,
		db:    make(chan int, 10),
	}
	return &r
}

type httpRequester struct {
	data             []byte
	client           fasthttp.Client
	method           string
	pattern          *regexp.Regexp
	patternFormatter string
	state            *OPState
}

func newHTTPRequester(state *OPState) *httpRequester {
	if config.url == "" {
		panic("Must specify url for requests")
	}
	requester := httpRequester{
		state: state,
	}
	requester.pattern = regexp.MustCompile(`(X{2,})`)
	length := len(requester.pattern.FindString(config.url))
	requester.patternFormatter = fmt.Sprintf("%%0%dd", length)
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

func (requester *httpRequester) request(channels *OPChannels, request *Request) {
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(requester.pattern.ReplaceAllString(config.url,
		fmt.Sprintf(requester.patternFormatter,
			request.num)))

	req.Header.SetMethodBytes([]byte(requester.method))
	req.Header.Set("Connection", "keep-alive")
	if requester.data != nil {
		req.SetBody(requester.data)
	}
	resp := fasthttp.AcquireResponse()
	start := time.Now()
	err := requester.client.Do(req, resp)
	timeSpent := time.Since(start)
	statusCode := resp.StatusCode()
	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

	if err != nil {
		p(requester.state.colored("E"))
		channels.responses <- &Response{request, timeSpent,
			fmt.Errorf("Bad request: %v", err)}
		return
	}

	switch statusCode {
	case fasthttp.StatusOK, fasthttp.StatusCreated:
		p(requester.state.colored("."))
		channels.responses <- &Response{request, timeSpent, nil}
	default:
		p(requester.state.colored("E"))
		channels.responses <- &Response{request, timeSpent, fmt.Errorf("Error: statusCode")}
	}
}
