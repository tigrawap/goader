package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"net/http"
	"os"
)

var (
	OpCounters  = make(map[string]*prometheus.CounterVec)
	OpDurations = make(map[string]prometheus.Histogram)
)

func initMetrics() {
	for _, op := range allMetaOps {
		counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "goader_" + string(op) + "_ops",
			Help: "Number of " + string(op) + " operations",
		},
			[]string{"status", "payload_size"},
		)
		prometheus.MustRegister(counterVec)
		OpCounters[string(op)] = counterVec

		duration := prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "goader_" + string(op) + "_duration_nanoseconds",
			Help:    "Duration of " + string(op) + " operations in nanoseconds",
			Buckets: prometheus.ExponentialBuckets(64, 2, 25), // Adjust buckets as needed
		})
		prometheus.MustRegister(duration)
		OpDurations[string(op)] = duration
	}
}

func setMetrics() {
	if !config.enablePrometheusMetrics {
		return
	}

	// Create a new HTTP server to serve metrics
	http.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		DisableCompression: true,
	}))
	initMetrics()

	go func() {
		http.ListenAndServe(":8090", nil)
	}()
}

func dumpMetrics() {
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		fmt.Println("Error gathering metrics:", err)
		return
	}

	for _, m := range metrics {
		encoder := expfmt.NewEncoder(os.Stdout, expfmt.FmtText)
		err := encoder.Encode(m)
		if err != nil {
			fmt.Println("Error encoding metric:", err)
		}
	}
}
