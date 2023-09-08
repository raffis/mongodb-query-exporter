package v1

import (
	"context"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raffis/mongodb-query-exporter/v5/internal/collector"
	"github.com/raffis/mongodb-query-exporter/v5/internal/config"
	"github.com/raffis/mongodb-query-exporter/v5/internal/x/zap"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Configuration v1.0 format
type Config struct {
	MongoDB  MongoDB
	Bind     string
	LogLevel string
	Metrics  []*Metric
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

// Metric defines an exported metric from a MongoDB aggregation pipeline
type Metric struct {
	Cache         int64
	Mode          string
	Database      string
	Collection    string
	Pipeline      string
	Name          string
	Type          string
	Help          string
	Value         string
	OverrideEmpty bool
	EmptyValue    int64
	ConstLabels   prometheus.Labels
	Labels        []string
}

// Get address where the http server should be bound to
func (conf *Config) GetBindAddr() string {
	return conf.Bind
}

// Get metrics path
func (conf *Config) GetMetricsPath() string {
	return "/metrics"
}

// Build a collector from a configuration v1.0 format and return an Exprter with that collector.
// Note the v1.0 config does not support multiple collectors, you may instead use the v2.0 format.
func (conf *Config) Build() (*collector.Collector, error) {
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
	env := os.Getenv("MDBEXPORTER_SERVER_0_MONGODB_URI")

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
			DefaultCache:      time.Duration(conf.MongoDB.DefaultInterval) * time.Second,
			DefaultDatabase:   conf.MongoDB.DefaultDatabase,
			DefaultCollection: conf.MongoDB.DefaultCollection,
		}),
		collector.WithLogger(l.Sugar()),
		collector.WithCounter(config.Counter),
	)

	err = c.RegisterServer("main", d)
	if err != nil {
		return c, err
	}

	if len(conf.Metrics) == 0 {
		l.Sugar().Warn("no metrics have been configured")
	}

	for _, metric := range conf.Metrics {
		err := c.RegisterAggregation(&collector.Aggregation{
			Cache:      time.Duration(metric.Cache) * time.Second,
			Mode:       metric.Mode,
			Database:   metric.Database,
			Collection: metric.Collection,
			Pipeline:   metric.Pipeline,
			Metrics: []*collector.Metric{
				{
					Name:          metric.Name,
					Type:          metric.Type,
					Help:          metric.Help,
					Value:         metric.Value,
					OverrideEmpty: metric.OverrideEmpty,
					EmptyValue:    metric.EmptyValue,
					ConstLabels:   metric.ConstLabels,
					Labels:        metric.Labels,
				},
			},
		})

		if err != nil {
			return c, err
		}
	}

	return c, nil
}
