package config

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raffis/mongodb-query-exporter/v5/internal/collector"
)

// Config defaults
const (
	DefaultServerName   = "main"
	DefaultMongoDBURI   = "mongodb://localhost:27017"
	DefaultMetricsPath  = "/metrics"
	DefaultBindAddr     = ":9412"
	DefaultQueryTimeout = 10 * time.Second
	HealthzPath         = "/healthz"
	DefaultLogEncoder   = "json"
	DefaultLogLevel     = "warn"
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
	[]string{"aggregation", "server", "result"},
)
