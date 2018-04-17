package main

import (
	"bytes"
	"html/template"
	"io/ioutil"
	"sort"
	"time"
)

type RequestTimes struct {
	Start   time.Time
	Latency time.Duration
	Success bool
}

type HTMLResult struct {
	TimeLine   ResultingTimeLine
	MaxLatency time.Duration
}

type TimeLineResult struct {
	Times         RequestTimes
	Opname        string
	op            op
	Color         string
	Width         float64
	ShowTimestamp bool
}

type TimeLine []RequestTimes
type ResultingTimeLine []TimeLineResult

func (a ResultingTimeLine) Len() int           { return len(a) }
func (a ResultingTimeLine) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ResultingTimeLine) Less(i, j int) bool { return a[i].Times.Start.Before(a[j].Times.Start) }

func (r RequestTimes) LessThan(b RequestTimes) bool {
	return r.Start.Before(b.Start)
}

func (i RequestTimes) EqualTo(j RequestTimes) bool {
	return i.Start.Equal(j.Start)
}

const TIMELINE_TEMPLATE = `
<html>
<h3> 100 width % = {{.MaxLatency}}</h3>
<div style='overflow:hidden; max-width:1024px; margin:0 auto; border:1px dotted'>
	{{ range .TimeLine }}
		{{ if .ShowTimestamp }}
		<div style='width:100%; float:left'>{{.Times.Start}}</div>
		{{ end}}
		<div style='height:1px; width:{{.Width}}%;background:{{.Color}}; float:left;{{ if .Times.Success}}{{else}};border-bottom:1px solid red{{end}}'
		></div>
		<div style='height:1px; width:100%; float:left'></div>
	{{ end }}
	</div>
</div>
<html>
`

func buildTimeline(states ...*OPState) {
	if config.timelineFile == EmptyString {
		return
	}
	r := ResultingTimeLine{}

	var maxDuration time.Duration
	for _, state := range states {
		for _, times := range state.timeline {
			if times.Latency > maxDuration {
				maxDuration = times.Latency
			}
		}
	}
	for _, state := range states {
		for _, times := range state.timeline {
			width := float64(int64(times.Latency)) / float64(int64(maxDuration)) * 100
			if width < 1 {
				width = 1
			}
			result := TimeLineResult{
				Times:  times,
				Opname: state.name,
				op:     state.op,
				Color:  state.color,
				Width:  width,
			}
			//fmt.Println(result)
			r = append(r, result)
		}
	}
	sort.Sort(r)
	lastTimeStamp := time.Time{}
	for i := range r {
		res := &r[i]
		if res.Times.Start.Sub(lastTimeStamp) > time.Duration(300*time.Millisecond) {
			lastTimeStamp = res.Times.Start
			res.ShowTimestamp = true
		} else {
			res.ShowTimestamp = false
		}
	}
	//fmt.Println(r)
	tmpl, err := template.New("main").Parse(TIMELINE_TEMPLATE)
	if err != nil {
		config.output.reportError(err.Error())
	}
	b := bytes.Buffer{}
	err = tmpl.Execute(&b, HTMLResult{r, maxDuration})
	if err != nil {
		config.output.reportError(err.Error())
	}
	//	b, err := json.Marshal(r)
	//	fmt.Println(r)
	//	fmt.Printf("%v\n", string(b[:]))
	//	if err != nil {
	//		config.output.reportError(err.Error())
	//		return
	//	}
	err = ioutil.WriteFile(config.timelineFile, b.Bytes(), 0644)
	if err != nil {
		config.output.reportError(err.Error())
	}
}
