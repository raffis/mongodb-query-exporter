package v2

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/raffis/mongodb-query-exporter/collector"
	"github.com/raffis/mongodb-query-exporter/config"
	"github.com/raffis/mongodb-query-exporter/x/zap"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Configuration v2.0 format
type Config struct {
	Bind       string
	Log        zap.Config
	Collectors []*Collector
}

// Collector configurations
// Holds a MongoDB configuration as well as a list of Metrics to be generated
type Collector struct {
	MongoDB *MongoDB
	Metrics []*collector.Metric
}

// MongoDB client options
type MongoDB struct {
	URI               string
	QueryTimeout      time.Duration
	MaxConnections    int32
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

// Build collectors from a configuration v2.0 format and return a collection of
// all configured collectors
func (conf *Config) Build() (config.Exporter, error) {
	e := &Exporter{}
	l, err := zap.New(conf.Log)

	if err != nil {
		return nil, err
	}

	if conf.Bind == "" {
		conf.Bind = ":9412"
	}

	l.Sugar().Infof("will listen on %s", conf.Bind)

	for id, srv := range conf.Collectors {
		env := os.Getenv(fmt.Sprintf("MDBEXPORTER_COLLECTORS_%d_MONGODB_URI", id))

		if env != "" {
			srv.MongoDB.URI = env
		}

		if srv.MongoDB.URI == "" {
			srv.MongoDB.URI = "mongodb://localhost:27017"
		}

		opts := options.Client().ApplyURI(srv.MongoDB.URI)
		l.Sugar().Infof("use mongodb hosts %#v", opts.Hosts)
		var err error

		d := &collector.MongoDBDriver{}
		err = d.Connect(context.TODO(), opts)
		if err != nil {
			panic(err)
		}

		if srv.MongoDB.QueryTimeout == 0 {
			srv.MongoDB.QueryTimeout = 10
		}

		c := collector.New(
			collector.WithConfig(&collector.Config{
				QueryTimeout:      srv.MongoDB.QueryTimeout,
				DefaultInterval:   srv.MongoDB.DefaultInterval,
				DefaultDatabase:   srv.MongoDB.DefaultDatabase,
				DefaultCollection: srv.MongoDB.DefaultCollection,
			}),
			collector.WithLogger(l.Sugar()),
			collector.WithDriver(d),
		)

		e.collectors = append(e.collectors, c)

		for _, metric := range srv.Metrics {
			go func(metric *collector.Metric) {
				c.WithMetric(metric)
			}(metric)
		}
	}

	return e, nil
}
