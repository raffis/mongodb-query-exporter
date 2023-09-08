package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"time"

	"github.com/raffis/mongodb-query-exporter/v5/internal/collector"
	"github.com/raffis/mongodb-query-exporter/v5/internal/config"
	v1 "github.com/raffis/mongodb-query-exporter/v5/internal/config/v1"
	v2 "github.com/raffis/mongodb-query-exporter/v5/internal/config/v2"
	v3 "github.com/raffis/mongodb-query-exporter/v5/internal/config/v3"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	configPath    string
	logLevel      string
	logEncoding   string
	bind          string
	uri           string
	metricsPath   string
	queryTimeout  time.Duration
	srv           *http.Server
	promCollector *collector.Collector
)

func init() {
	flag.StringVarP(&uri, "uri", "u", config.DefaultMongoDBURI, "MongoDB URI (default is mongodb://localhost:27017). Use MDBEXPORTER_SERVER_%d_MONGODB_URI envs if you target multiple server")
	flag.StringVarP(&configPath, "file", "f", "", "config file (default is $HOME/.mongodb_query_exporter/config.yaml)")
	flag.StringVarP(&logLevel, "log-level", "l", config.DefaultLogLevel, "Define the log level (default is warning) [debug,info,warn,error]")
	flag.StringVarP(&logEncoding, "log-encoding", "e", config.DefaultLogEncoder, "Define the log format (default is json) [json,console]")
	flag.StringVarP(&bind, "bind", "b", config.DefaultBindAddr, "Address to bind http server (default is :9412)")
	flag.StringVarP(&metricsPath, "path", "p", config.DefaultMetricsPath, "Metric path (default is /metrics)")
	flag.DurationVarP(&queryTimeout, "query-timeout", "t", config.DefaultQueryTimeout, "Timeout for MongoDB queries")

	_ = viper.BindPFlag("log.level", flag.Lookup("log-level"))
	_ = viper.BindPFlag("log.encoding", flag.Lookup("log-encoding"))
	_ = viper.BindPFlag("bind", flag.Lookup("bind"))
	_ = viper.BindPFlag("metricsPath", flag.Lookup("path"))
	_ = viper.BindPFlag("mongodb.uri", flag.Lookup("uri"))
	_ = viper.BindPFlag("mongodb.queryTimeout", flag.Lookup("query-timeout"))
	_ = viper.BindEnv("mongodb.uri", "MDBEXPORTER_MONGODB_URI")
	_ = viper.BindEnv("global.queryTimeout", "MDBEXPORTER_MONGODB_QUERY_TIMEOUT")
	_ = viper.BindEnv("log.level", "MDBEXPORTER_LOG_LEVEL")
	_ = viper.BindEnv("log.encoding", "MDBEXPORTER_LOG_ENCODING")
	_ = viper.BindEnv("bind", "MDBEXPORTER_BIND")
	_ = viper.BindEnv("metricsPath", "MDBEXPORTER_METRICSPATH")
}

func main() {
	flag.Parse()
	initConfig()

	c, conf, err := buildCollector()
	if err != nil {
		panic(err)
	}

	prometheus.MustRegister(c)
	promCollector = c
	_ = c.StartCacheInvalidator()
	srv = buildHTTPServer(prometheus.DefaultGatherer, conf)
	err = srv.ListenAndServe()

	// Only panic if we have a net error
	if _, ok := err.(*net.OpError); ok {
		panic(err)
	} else {
		os.Stderr.WriteString(err.Error() + "\n")
	}
}

func buildCollector() (*collector.Collector, config.Config, error) {
	var configVersion float32
	err := viper.UnmarshalKey("version", &configVersion)
	if err != nil {
		panic(err)
	}

	var conf config.Config
	switch configVersion {
	case 3.0:
		conf = &v3.Config{}

	case 2.0:
		conf = &v2.Config{}

	default:
		conf = &v1.Config{}
	}

	err = viper.Unmarshal(&conf)
	if err != nil {
		panic(err)
	}

	if os.Getenv("MDBEXPORTER_MONGODB_URI") != "" {
		os.Setenv("MDBEXPORTER_SERVER_0_MONGODB_URI", os.Getenv("MDBEXPORTER_MONGODB_URI"))
	}

	if uri != "" && uri != "mongodb://localhost:27017" {
		os.Setenv("MDBEXPORTER_SERVER_0_MONGODB_URI", uri)
	}

	c, err := conf.Build()
	return c, conf, err
}

// Run executes a blocking http server. Starts the http listener with the metrics and healthz endpoints.
func buildHTTPServer(reg prometheus.Gatherer, conf config.Config) *http.Server {
	mux := http.NewServeMux()

	if conf.GetMetricsPath() != "/" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, fmt.Sprintf("Use the %s endpoint", conf.GetMetricsPath()), http.StatusOK)
		})
	}

	mux.HandleFunc(config.HealthzPath, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) })
	mux.HandleFunc(conf.GetMetricsPath(), func(w http.ResponseWriter, r *http.Request) {
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})

	srv := http.Server{Addr: conf.GetBindAddr(), Handler: mux}
	return &srv
}

func initConfig() {
	envPath := os.Getenv("MDBEXPORTER_CONFIG")

	if configPath != "" {
		// Use config file from the flag.
		viper.SetConfigFile(configPath)
	} else if envPath != "" {
		// Use config file from env.
		viper.SetConfigFile(envPath)
	} else {
		// Find home directory.
		usr, err := user.Current()
		if err == nil {
			viper.AddConfigPath(usr.HomeDir + "/.mongodb_query_exporter")
		}

		// System wide config
		viper.AddConfigPath("/etc/mongodb-query-exporter")
		viper.AddConfigPath("/etc/mongodb_query_exporter")
	}

	viper.SetConfigType("yaml")
	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
}
