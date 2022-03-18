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
	Bind        string
	MetricsPath string
	Log         zap.Config
	Global      Global
	Servers     []*Server
	Metrics     []*collector.Metric
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

// Config defaults
const (
	DefaultServerName   = "localhost:27017"
	DefaultMongoDBURI   = "mongodb://localhost:27017"
	DefaultMetricsPath  = "/metrics"
	DefaultBindAddr     = ":9412"
	DefaultQUeryTimeout = 10
	HealthzPath         = "/healthz"
	DefaultLogEncoder   = "json"
	DefaultLogLevel     = "warn"
)

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
		conf.Log.Encoding = DefaultLogEncoder
	}

	if conf.Log.Level == "" {
		conf.Log.Level = DefaultLogLevel
	}

	l, err := zap.New(conf.Log)
	if err != nil {
		return nil, err
	}

	if conf.MetricsPath == "" {
		conf.MetricsPath = DefaultMetricsPath
	} else if conf.MetricsPath == HealthzPath {
		return nil, fmt.Errorf("%s not allowed as metrics path", HealthzPath)
	}

	if conf.Bind == "" {
		conf.Bind = DefaultBindAddr
	}

	l.Sugar().Infof("will listen on %s", conf.Bind)

	if conf.Global.QueryTimeout == 0 {
		conf.Global.QueryTimeout = 10
	}

	if len(conf.Servers) == 0 {
		conf.Servers = append(conf.Servers, &Server{
			Name: DefaultServerName,
		})
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
			srv.URI = DefaultMongoDBURI
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

	if len(conf.Metrics) == 0 {
		l.Sugar().Warn("no metrics have been configured")
	}

	for _, metric := range conf.Metrics {
		err := c.RegisterMetric(metric)
		if err != nil {
			return c, err
		}
	}

	return c, nil
}
