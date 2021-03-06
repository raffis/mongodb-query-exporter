package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/user"

	"github.com/raffis/mongodb-query-exporter/collector"
	"github.com/raffis/mongodb-query-exporter/config"
	v1 "github.com/raffis/mongodb-query-exporter/config/v1"
	v2 "github.com/raffis/mongodb-query-exporter/config/v2"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	c            *collector.Collector
	configPath   string
	logLevel     string
	logEncoding  string
	bind         string
	uri          string
	metricsPath  string
	queryTimeout int
	rootCmd      = &cobra.Command{
		Use:   "mongodb_query_exporter",
		Short: "MongoDB aggregation exporter for prometheus",
		Long:  `Export aggregations from MongoDB as prometheus metrics.`,
		Run: func(cmd *cobra.Command, args []string) {
			var configVersion float32
			err := viper.UnmarshalKey("version", &configVersion)
			if err != nil {
				panic(err)
			}

			var conf config.Config
			switch configVersion {
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

			c, err = conf.Build()
			if err != nil {
				panic(err)
			}

			prometheus.MustRegister(c)
			c.StartCacheInvalidator()
			serve(prometheus.DefaultGatherer, conf)
		},
	}
)

// Run executes a blocking http server. Starts the http listener with the metrics and healthz endpoints.
func serve(reg prometheus.Gatherer, conf config.Config) {
	if conf.GetMetricsPath() != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, fmt.Sprintf("Use the %s endpoint", conf.GetMetricsPath()), http.StatusOK)
		})
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) })
	http.HandleFunc(conf.GetMetricsPath(), func(w http.ResponseWriter, r *http.Request) {
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	})

	err := http.ListenAndServe(conf.GetBindAddr(), nil)

	// If the port is already in use or another fatal error panic
	if err != nil {
		panic(err)
	}
}

// Executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	//deprecated, use -f
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "[Deprecated, use -f/--file] config file (default is $HOME/.mongodb_query_exporter/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&uri, "uri", "u", "mongodb://localhost:27017", "[Deprecated, use the config file or MDBEXPORTER_SERVER_0_MONGODB_URI env] MongoDB URI (default is mongodb://localhost:27017)")

	rootCmd.PersistentFlags().StringVarP(&configPath, "file", "f", "", "config file (default is $HOME/.mongodb_query_exporter/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Define the log level (default is info) [debug,info,warning,error]")
	rootCmd.PersistentFlags().StringVarP(&logEncoding, "log-encoding", "e", "json", "Define the log format (default is json) [json,console]")
	rootCmd.PersistentFlags().StringVarP(&bind, "bind", "b", ":9412", "Address to bind http server (default is :9412)")
	rootCmd.PersistentFlags().StringVarP(&metricsPath, "path", "p", "/metrics", "Metric path (default is /metrics)")
	rootCmd.PersistentFlags().IntVarP(&queryTimeout, "query-timeout", "t", 10, "Timeout for MongoDB queries")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.encoding", rootCmd.PersistentFlags().Lookup("log-encoding"))
	viper.BindPFlag("bind", rootCmd.PersistentFlags().Lookup("bind"))
	viper.BindPFlag("metricsPath", rootCmd.PersistentFlags().Lookup("path"))
	viper.BindPFlag("mongodb.uri", rootCmd.PersistentFlags().Lookup("uri"))
	viper.BindPFlag("mongodb.queryTimeout", rootCmd.PersistentFlags().Lookup("query-timeout"))
	viper.BindEnv("mongodb.uri", "MDBEXPORTER_MONGODB_URI")
	viper.BindEnv("global.queryTimeout", "MDBEXPORTER_MONGODB_QUERY_TIMEOUT")
	viper.BindEnv("log.level", "MDBEXPORTER_LOG_LEVEL")
	viper.BindEnv("log.encoding", "MDBEXPORTER_LOG_ENCODING")
	viper.BindEnv("bind", "MDBEXPORTER_BIND")
	viper.BindEnv("metricsPath", "MDBEXPORTER_METRICSPATH")
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
		if err != nil {
			panic(err)
		}

		// System wide config
		viper.AddConfigPath("/etc/mongodb_query_exporter")
		// Search config in home directory with name ".mongodb_query_exporter" (without extension).
		viper.AddConfigPath(usr.HomeDir + "/.mongodb_query_exporter")
		//config file name without extension
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}
}
