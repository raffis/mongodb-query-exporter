package config

import "github.com/raffis/mongodb-query-exporter/collector"

// Collector
type Collector interface {
	WithMetric(*collector.Metric)
}

// Exporter holds a collection of Collectors
type Exporter interface {
	Collectors() []Collector
}

// A configuration format to build a Collector from
type Config interface {
	GetBindAddr() string
	Build() (Exporter, error)
}
