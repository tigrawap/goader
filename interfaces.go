package main

import "github.com/valyala/fasthttp"

// Emitter produce requests, they should be high efficient,
// since called after every completed request and periodically
type Emitter interface {
	emitRequests(state *OPState)
}

//URLFormatter alters user supplied url string
type URLFormatter interface {
	format(requestNum int64) string
}

//HTTPAuther alters http requests with authentication headers
type HTTPAuther interface {
	sign(r *fasthttp.Request)
}

//Adjuster should decide whether change throughput based on response
type Adjuster interface {
	adjust(response *Response)
}

//Requester does actual requests to server/fs
type Requester interface {
	request(channel chan *Response, request *Request)
}

//Target supplies requester with num of file/template
type Target interface {
	get() string
}

//Output presents result in human or variable machine readable forms
type Output interface {
	progress(s string)
	reportError(s string)
	report(s string)
	printResults(result *Results)
}

type S3Auther interface {
	sign(r *fasthttp.Request)
}

type PayloadGetter interface {
	Get() []byte
	GetFull() []byte
	GetLength() int64
}