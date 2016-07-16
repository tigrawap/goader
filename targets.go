package main

import (
	"fmt"
	"math/rand"
	"regexp"
	"time"
)

const LETTERS = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const NUMS = "0123456789"

func randString(n int, letters string) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

//IncrementalTarget increases number of request one by one
type TemplatedTarget struct {
	targets   chan string
	formatter URLFormatter
}

type templateFormatter struct {
	base            string
	incremental     *regexp.Regexp
	random          *regexp.Regexp
	randomNum       *regexp.Regexp
	intFormat       string
	randomLength    int
	randomNumLength int
	incLength       int
}

func newTemplateFormatter(url string) *templateFormatter {
	if url == "" {
		panic("Must specify url for requests")
	}
	formatter := templateFormatter{
		base:        url,
		incremental: regexp.MustCompile(`(X{2,})`),
		random:      regexp.MustCompile(`(R{2,})`),
		randomNum:   regexp.MustCompile(`(N{2,})`),
	}
	length := len(formatter.incremental.FindString(formatter.base))
	formatter.intFormat = fmt.Sprintf("%%0%dd", length)
	formatter.incLength = len(formatter.incremental.FindString(formatter.base))
	formatter.randomLength = len(formatter.random.FindString(formatter.base))
	formatter.randomNumLength = len(formatter.randomNum.FindString(formatter.base))
	return &formatter
}

func (f *templateFormatter) format(requestNum int64) string {
	r := f.base
	if f.incLength > 0 {
		r = f.incremental.ReplaceAllString(r, fmt.Sprintf(f.intFormat, requestNum))
	}
	if f.randomNumLength > 0 {
		r = f.randomNum.ReplaceAllString(r, randString(f.randomNumLength, NUMS))
	}
	if f.randomLength > 0 {
		r = f.random.ReplaceAllString(r, randString(f.randomLength, LETTERS))
	}
	return r
}

func newTemplatedTarget() *TemplatedTarget {
	i := TemplatedTarget{}
	i.formatter = newTemplateFormatter(config.url)
	go func() {
		n := int64(0)
		i.targets = make(chan string, 1000)
		for {
			n++
			i.targets <- i.formatter.format(n)
		}
	}()
	return &i
}

func (i *TemplatedTarget) get() string {
	return <-i.targets
}

// BoundTarget will set number of requests randomaly selected from bound slice
type BoundTarget struct {
	bound *[]string
}

func (b *BoundTarget) get() string {
	for {
		urls := *b.bound
		if len(urls) == 0 {
			time.Sleep(time.Millisecond)
			continue
		}
		return urls[rand.Intn(int(len(urls)))]
	}
}
