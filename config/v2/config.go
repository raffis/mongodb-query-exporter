package v2

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/raffis/mongodb-query-exporter/collector"
	"github.com/raffis/mongodb-query-exporter/config"
	"github.com/raffis/mongodb-query-exporter/x/zap"

	"go.mongodb.org/mongo-driver/mongo/options"
)

// Configuration v2.0 format
type Config struct {
	Bind    string
	Log     zap.Config
	Global  Global
	Servers []*Server
	Metrics []*collector.Metric
}

type Global struct {
	QueryTimeout      time.Duration
	MaxConnections    int32
	DefaultCache      int64
	DefaultMode       string
	DefaultDatabase   string
	DefaultCollection string
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

// Build collectors from a configuration v2.0 format and return a collection of
// all configured collectors
func (conf *Config) Build() (*collector.Collector, error) {
	l, err := zap.New(conf.Log)

	if err != nil {
		return nil, err
	}

	if conf.Bind == "" {
		conf.Bind = ":9412"
	}

	l.Sugar().Infof("will listen on %s", conf.Bind)

	if conf.Global.QueryTimeout == 0 {
		conf.Global.QueryTimeout = 10
	}

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
			srv.URI = "mongodb://localhost:27017"
		}

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

	for _, metric := range conf.Metrics {
		err := c.RegisterMetric(metric)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}
