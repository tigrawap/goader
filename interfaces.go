package main

// Emitter produce requests, they should be high efficient,
// since called after every completed request and periodically
type Emitter interface {
	emitRequests(channels *OPChannels, state *OPState)
}

//Adjuster should decide whether change throughput based on response
type Adjuster interface {
	adjust(response *Response)
}

//Requester does actual requests to server/fs
type Requester interface {
	request(channels *OPChannels, request *Request)
}

//Target supplies requester with num of file/template
type Target interface {
	get() int64
}
