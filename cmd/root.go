package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/user"

	"github.com/raffis/mongodb-query-exporter/config"
	v1 "github.com/raffis/mongodb-query-exporter/config/v1"
	v2 "github.com/raffis/mongodb-query-exporter/config/v2"
	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	configPath   string
	logLevel     string
	logEncoding  string
	bind         string
	uri          string
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

			_, err = conf.Build()
			if err != nil {
				panic(err)
			}

			serve(conf.GetBindAddr())
		},
	}
)

// Run executes a blocking http server. Starts the http listener with the /metrics endpoint
// and parses all configured metrics passed by config
func serve(addr string) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) })
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		promhttp.Handler().ServeHTTP(w, r)
	})

	err := http.ListenAndServe(addr, nil)

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
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "config file (default is $HOME/.mongodb_query_exporter/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Define the log level (default is info) [debug,info,warning,error]")
	rootCmd.PersistentFlags().StringVarP(&logEncoding, "log-encoding", "e", "json", "Define the log format (default is json) [json,console]")
	rootCmd.PersistentFlags().StringVarP(&bind, "bind", "b", ":9412", "Address to bind http server (default is :9412)")
	rootCmd.PersistentFlags().StringVarP(&uri, "uri", "u", "mongodb://localhost:27017", "MongoDB URI (default is mongodb://localhost:27017)")
	rootCmd.PersistentFlags().IntVarP(&queryTimeout, "query-timeout", "t", 10, "Timeout for MongoDB queries")
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.encoding", rootCmd.PersistentFlags().Lookup("log-encoding"))
	viper.BindPFlag("bind", rootCmd.PersistentFlags().Lookup("bind"))
	viper.BindPFlag("mongodb.uri", rootCmd.PersistentFlags().Lookup("uri"))
	viper.BindPFlag("mongodb.queryTimeout", rootCmd.PersistentFlags().Lookup("query-timeout"))
	viper.BindEnv("mongodb.uri", "MDBEXPORTER_MONGODB_URI")
	viper.BindEnv("mongodb.queryTimeout", "MDBEXPORTER_MONGODB_QUERY_TIMEOUT")
	viper.BindEnv("log.level", "MDBEXPORTER_LOG_LEVEL")
	viper.BindEnv("log.encoding", "MDBEXPORTER_LOG_ENCODING")
	viper.BindEnv("bind", "MDBEXPORTER_BIND")
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
			log.Error(err)
			return
		}

		// System wide config
		viper.AddConfigPath("/etc/mongodb_query_exporter")
		// Search config in home directory with name ".mongodb_query_exporter" (without extension).
		viper.AddConfigPath(usr.HomeDir + "/.mongodb_query_exporter")
		//config file name without extension
		viper.SetConfigName("config")
	}

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("failed to open config file %s\n", err)
	}
}
