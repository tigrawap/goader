package main

import (
	randc "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
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
	data      []byte
	client    fasthttp.Client
	method    string
	formatter *templateFormatter
	state     *OPState
}

type templateFormatter struct {
	base      string
	regex     *regexp.Regexp
	intFormat string
}

func newTemplateFormatter(url string) *templateFormatter {
	if url == "" {
		panic("Must specify url for requests")
	}
	formatter := templateFormatter{
		base:  url,
		regex: regexp.MustCompile(`(X{2,})`),
	}
	length := len(formatter.regex.FindString(formatter.base))
	formatter.intFormat = fmt.Sprintf("%%0%dd", length)
	return &formatter
}

func newHTTPRequester(state *OPState) *httpRequester {
	requester := httpRequester{
		state:     state,
		formatter: newTemplateFormatter(config.url),
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
	req.SetRequestURI(requester.formatter.regex.ReplaceAllString(config.url,
		fmt.Sprintf(requester.formatter.intFormat,
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
	state     *OPState
	formatter *templateFormatter
	data      []byte
}

func newDiskRequester(state *OPState) *diskRequester {
	r := diskRequester{
		state:     state,
		formatter: newTemplateFormatter(config.url),
	}
	if state.op == WRITE {
		r.data = make([]byte, config.bodySize)
		randc.Read(r.data)
	}
	return &r
}

func (requester *diskRequester) request(responses chan *Response, request *Request) {
	filename := requester.formatter.regex.ReplaceAllString(config.url,
		fmt.Sprintf(requester.formatter.intFormat,
			request.num))
	var err error
	start := time.Now()
	if requester.state.op == WRITE {
		err = ioutil.WriteFile(filename, requester.data, 0644)
	} else {
		_, err = ioutil.ReadFile(filename)
	}
	responses <- &Response{request, time.Since(start), err}
}
