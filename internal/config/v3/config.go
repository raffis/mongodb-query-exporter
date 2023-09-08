package v3

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/raffis/mongodb-query-exporter/v5/internal/collector"
	"github.com/raffis/mongodb-query-exporter/v5/internal/config"
	"github.com/raffis/mongodb-query-exporter/v5/internal/x/zap"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Configuration v3.0 format
type Config struct {
	Bind         string
	MetricsPath  string
	Log          zap.Config
	Global       Global
	Servers      []*Server
	Aggregations []*collector.Aggregation
}

// Global config
type Global struct {
	QueryTimeout      time.Duration
	MaxConnections    int32
	DefaultCache      time.Duration
	DefaultMode       string
	DefaultDatabase   string
	DefaultCollection string
}

// Aggregation defines what aggregation pipeline is executed on what servers
type Aggregation struct {
	Servers    []string
	Cache      time.Duration
	Mode       string
	Database   string
	Collection string
	Pipeline   string
	Metrics    []Metric
}

// Metric defines how a certain value is exported from a MongoDB aggregation
type Metric struct {
	Name          string
	Type          string
	Help          string
	Value         string
	OverrideEmpty bool
	EmptyValue    int64
	ConstLabels   prometheus.Labels
	Labels        []string
}

// MongoDB client options
type Server struct {
	Name string
	URI  string
}

// Get address where the http server should be bound to
func (conf *Config) GetBindAddr() string {
	return conf.Bind
}

// Get metrics path
func (conf *Config) GetMetricsPath() string {
	return conf.MetricsPath
}

// Build collectors from a configuration v2.0 format and return a collection of
// all configured collectors
func (conf *Config) Build() (*collector.Collector, error) {
	if conf.Log.Encoding == "" {
		conf.Log.Encoding = config.DefaultLogEncoder
	}

	if conf.Log.Level == "" {
		conf.Log.Level = config.DefaultLogLevel
	}

	l, err := zap.New(conf.Log)
	if err != nil {
		return nil, err
	}

	if conf.MetricsPath == "" {
		conf.MetricsPath = config.DefaultMetricsPath
	} else if conf.MetricsPath == config.HealthzPath {
		return nil, fmt.Errorf("%s not allowed as metrics path", config.HealthzPath)
	}

	if conf.Bind == "" {
		conf.Bind = config.DefaultBindAddr
	}

	l.Sugar().Infof("will listen on %s", conf.Bind)

	if conf.Global.QueryTimeout == 0 {
		conf.Global.QueryTimeout = config.DefaultQueryTimeout
	}

	if len(conf.Servers) == 0 {
		conf.Servers = append(conf.Servers, &Server{
			Name: config.DefaultServerName,
		})
	}

	config.Counter.Reset()
	c := collector.New(
		collector.WithConfig(&collector.Config{
			QueryTimeout:      conf.Global.QueryTimeout,
			DefaultCache:      conf.Global.DefaultCache,
			DefaultMode:       conf.Global.DefaultMode,
			DefaultDatabase:   conf.Global.DefaultDatabase,
			DefaultCollection: conf.Global.DefaultCollection,
		}),
		collector.WithLogger(l.Sugar()),
		collector.WithCounter(config.Counter),
	)

	for id, srv := range conf.Servers {
		env := os.Getenv(fmt.Sprintf("MDBEXPORTER_SERVER_%d_MONGODB_URI", id))

		if env != "" {
			srv.URI = env
		}

		if srv.URI == "" {
			srv.URI = config.DefaultMongoDBURI
		}

		srv.URI = os.ExpandEnv(srv.URI)
		opts := options.Client().ApplyURI(srv.URI)
		l.Sugar().Infof("use mongodb hosts %#v", opts.Hosts)

		var err error
		name := srv.Name
		if name == "" {
			name = strings.Join(opts.Hosts, ",")
		}

		d := &collector.MongoDBDriver{}
		err = d.Connect(context.TODO(), opts)
		if err != nil {
			panic(err)
		}

		err = c.RegisterServer(name, d)
		if err != nil {
			return c, err
		}
	}

	if len(conf.Aggregations) == 0 {
		l.Sugar().Warn("no aggregations have been configured")
	}

	for i, aggregation := range conf.Aggregations {
		opts := &collector.Aggregation{
			Servers:    aggregation.Servers,
			Cache:      aggregation.Cache,
			Mode:       aggregation.Mode,
			Database:   aggregation.Database,
			Collection: aggregation.Collection,
			Pipeline:   aggregation.Pipeline,
		}

		for _, metric := range aggregation.Metrics {
			opts.Metrics = append(opts.Metrics, &collector.Metric{
				Name:          metric.Name,
				Type:          metric.Type,
				Help:          metric.Help,
				Value:         metric.Value,
				OverrideEmpty: metric.OverrideEmpty,
				EmptyValue:    metric.EmptyValue,
				ConstLabels:   metric.ConstLabels,
				Labels:        metric.Labels,
			})
		}

		if len(conf.Aggregations) == 0 {
			l.Sugar().Warn("no metrics have been configured for aggregation_%d", i)
		}

		err := c.RegisterAggregation(aggregation)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}
