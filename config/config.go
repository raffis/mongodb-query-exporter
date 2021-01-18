package config

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/raffis/mongodb-query-exporter/collector"
)

// A configuration format to build a Collector from
type Config interface {
	GetBindAddr() string
	GetMetricsPath() string
	Build() (*collector.Collector, error)
}

var Counter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "mongodb_query_exporter_query_total",
		Help: "How many MongoDB queries have been processed, partitioned by metric, server and status",
	},
	[]string{"metric", "server", "result"},
)
