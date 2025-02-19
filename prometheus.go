package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"net/http"
	"os"
)

var (
	OpCounters  = make(map[string]*prometheus.CounterVec)
	OpDurations = make(map[string]prometheus.Histogram)
)

func initMetrics() {
	metrics := allMetaOps
	metrics = append(metrics, "sleep", "null")
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
		skip := true
		for _, metric := range m.Metric {
			switch *m.Type {
			case io_prometheus_client.MetricType_COUNTER:
				if metric.Counter.GetValue() != 0 {
					skip = false
				}
			case io_prometheus_client.MetricType_GAUGE:
				if metric.Gauge.GetValue() != 0 {
					skip = false
				}
			case io_prometheus_client.MetricType_HISTOGRAM:
				if metric.Histogram.GetSampleCount() != 0 {
					skip = false
				}
			case io_prometheus_client.MetricType_SUMMARY:
				if metric.Summary.GetSampleCount() != 0 {
					skip = false
				}
			case io_prometheus_client.MetricType_UNTYPED:
				if metric.Untyped.GetValue() != 0 {
					skip = false
				}
			}
		}

		if skip {
			continue
		}

		encoder := expfmt.NewEncoder(os.Stdout, expfmt.FmtText)
		err := encoder.Encode(m)
		if err != nil {
			fmt.Println("Error encoding metric:", err)
		}
	}
}
