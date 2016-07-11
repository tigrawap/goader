### Goader intro

Why another benchmarks utility?
All known to me utilities have several of this flows:

- Too complex, often file based configuration to achieve wanted by everyone required effect
- Do not properly simulate real scenarios

What I mean by real scenarios:  
I)  At some point latency of your services grows, 500ms per request is not fun. 
So you try to optimize things or even change backend. 
Problem is, that all benchmarks have `concurrent requests` settings, but you dont really have this information  
Information that you have - you got N requests per second and latency rised to X and X is not good.
So when optimizing things it's important to know, what you will get with specific number of evenly distributed requests per second, not concurrent requests

II) You got your service up, you truly beleive, that latency over 200ms is not usable. How many requests per second can it hold to maintain such latency? Yet again, `concurrent requests` does not answer this question

While both this scenarios is good enough reason for this benchmark utility to exist, they can be more complex, like 10 file PUTS, 100 GETS per second, or search for maximum concurrent threads with specific GET to PUT ration

### Features
- Extendable architecture with support of multiple request engines
- Simple url template, http://localhost/post/XXXXX. XXXXX will be replaced by incremental integers, (number of Xs>2)
- Supports different output formatters, atm human|readable

#####Request engines  
`--requests-engine=`  
- `null` Does nothing, useful for testing utility itself. At the moment reaches 700k requests per/sec  
- `sleep` Requster. For testing, sleeps instead of making real requests, also has semaphore of 10 concurrent connections, this emulates database which is bottleneck in the test and often in real life 
- `upload` Uploads (by PUT) and GET files (done, default engine).  
- `http` Planned. Simple single variable method http tester. Planned to support url template or weighted urls files list   
- `s3` planned, to test self hosted object storage with s3 interface  
- `disk` planned, should support variable block size and O_DIRECT mode  

##### Output formatters
`--output=`  
- `human` Default, human readable output, also displays real time progress  
- `json` Outputs results as json for automatic use by other processes  
   
### Examples  
`./goader -rps=30 -wps=30 --requests-engine=upload -url=http://127.0.0.1/post/XXXXX`  
Will maintain 30 writes, 30 reads per second, aborts if cannot maintain such rate, incrementaly increases url(http://127.0.0.1/post/00001, http://127.0.0.1/post/00002 ...)

`./goader --max-latency=300ms --requests-engine=upload -url="http://127.0.0.1/post/XXX"`
Will increase load gradually until reaching specified latency, useful for determining maximum throughput

`./goader --max-latency=300ms -rt=0 --requests-engine=upload -url="http://127.0.0.1/post/XXXXX"`
Same, but uploads only, no GETs

`./goader -rpt=10 --max-latency=300ms --requests-engine=upload -url="http://127.0.0.1/post/XXXXX"`
Will search for best PUT latency with read to write ratio 10, defaults to 1.
Maximum PUTs threads also limited by max-channels param, defaults to 500

`./goader -rt=200 -wt=30 --requests-engine=upload -url="http://127.0.0.1/post/XXXXX"`
Will maintain 200 read, 30 write threads

`./goader -rt=200 -wt=0 --requests-engine=upload -url="http://127.0.0.1/post/XXXXX"`
Same, reads only.

In all load patterns if both reads and writes issued reads will use previously written data, if only read pattern used - then incremental filenames (or from urls list file), with reads only it is expected for files/urls to be there

##### Example output
```
go run src/github.com/tigrawap/goader/*.go  --requests-engine=sleep --max-requests 500000  -wps=60 -rps=90                               [13:15:25]
.....................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................^C
 Stopped by user
Reads:
Average response time: 102.912582ms
Percentile  95 193.04865ms
Percentile  90 183.255368ms
Percentile  80 163.292791ms
Percentile  70 143.008297ms
Percentile  60 123.016042ms
Percentile  50 101.368878ms
Percentile  40 80.386543ms
Percentile  30 64.053213ms
Total requests: 858
Total errors: 0
Average OP/s:  89


Writes:
Average response time: 96.271904ms
Percentile  95 188.017613ms
Percentile  90 176.071752ms
Percentile  80 154.010595ms
Percentile  70 135.032065ms
Percentile  60 118.022375ms
Percentile  50 100.005855ms
Percentile  40 77.007488ms
Percentile  30 56.048966ms
Total requests: 573
Total errors: 0
Average OP/s:  60
```





