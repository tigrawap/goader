package main

import "github.com/valyala/fasthttp"

// Emitter produce requests, they should be high efficient,
// since called after every completed request and periodically
type Emitter interface {
	emitRequests(state *OPState)
}

//URLFormatter alters user supplied url string
type URLFormatter interface {
	format(url string, requestNum int64) string
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
	get() int64
}

//Output presents result in human or variable machine readable forms
type Output interface {
	progress(s string)
	error(s string)
	report(s string)
	printResults(result *Results)
}
