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

type sleepRequster struct {
	state *OPState
}

func (requester *sleepRequster) request(channels *OPChannels, request *Request) {
	if rand.Intn(10000)-requester.state.inFlight < 0 {
		print("Error")
		channels.responses <- &Response{request, 0, errors.New("Bad response")}
		return
	}
	print(".")
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
	resp := fasthttp.AcquireResponse()
	start := time.Now()
	err := requester.client.Do(req, resp)
	timeSpent := time.Since(start)
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
