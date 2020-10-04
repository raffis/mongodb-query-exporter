package v1

import (
	"context"
	"os"
	"time"

	"github.com/raffis/mongodb-query-exporter/collector"
	"github.com/raffis/mongodb-query-exporter/config"
	"github.com/raffis/mongodb-query-exporter/x/zap"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Configuration v1.0 format
type Config struct {
	MongoDB  *MongoDB
	Bind     string
	LogLevel string
	Metrics  []*collector.Metric
}

// MongoDB client options
type MongoDB struct {
	URI               string
	MaxConnections    int32
	ConnectionTimeout time.Duration
	DefaultInterval   int64
	DefaultDatabase   string
	DefaultCollection string
}

// Holds collectors
type Exporter struct {
	collectors []config.Collector
}

// Return list of collectors
func (exporter *Exporter) Collectors() []config.Collector {
	return exporter.collectors
}

// Get address where the http server should be bound to
func (conf *Config) GetBindAddr() string {
	return conf.Bind
}

// Build a collector from a configuration v1.0 format and return an Exprter with that collector.
// Note the v1.0 config does not support multiple collectors, you may instead use the v2.0 format.
func (conf *Config) Build() (config.Exporter, error) {
	e := &Exporter{}
	l, err := zap.New(zap.Config{
		Encoding: "console",
		Level:    conf.LogLevel,
	})

	if err != nil {
		return nil, err
	}

	if conf.Bind == "" {
		conf.Bind = ":9412"
	}

	l.Sugar().Infof("will listen on %s", conf.Bind)
	env := os.Getenv("MDBEXPORTER_COLLECTORS_0_MONGODB_URI")

	if env != "" {
		conf.MongoDB.URI = env
	}

	if conf.MongoDB.URI == "" {
		conf.MongoDB.URI = "mongodb://localhost:27017"
	}

	opts := options.Client().ApplyURI(conf.MongoDB.URI)
	l.Sugar().Infof("use mongodb hosts %#v", opts.Hosts)
	l.Sugar().Debug("use mongodb connection context timout of %d", conf.MongoDB.ConnectionTimeout*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), conf.MongoDB.ConnectionTimeout*time.Second)
	defer cancel()
	d := &collector.MongoDBDriver{}
	err = d.Connect(ctx, opts)
	if err != nil {
		panic(err)
	}

	c := collector.New(
		collector.WithConfig(&collector.Config{
			QueryTimeout:      conf.MongoDB.ConnectionTimeout,
			DefaultInterval:   conf.MongoDB.DefaultInterval,
			DefaultDatabase:   conf.MongoDB.DefaultDatabase,
			DefaultCollection: conf.MongoDB.DefaultCollection,
		}),
		collector.WithLogger(l.Sugar()),
		collector.WithDriver(d),
	)

	e.collectors = append(e.collectors, c)

	for _, metric := range conf.Metrics {
		go func(metric *collector.Metric) {
			c.WithMetric(metric)
		}(metric)
	}

	return e, nil
}
