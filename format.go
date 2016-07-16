package main

import (
	"bytes"
	"fmt"
	"time"
)

func TimesList(ss []time.Duration) string {
	var b bytes.Buffer
	for _, s := range ss {
		fmt.Fprintf(&b, "%v ", s)
	}
	return b.String()
}
