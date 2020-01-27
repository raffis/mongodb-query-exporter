package cmd

import (
	"os/user"

	"../collector"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	configPath string
	logLevel   string
	bind       string
	uri        string
	timeout    int
	rootCmd    = &cobra.Command{
		Use:   "mongodb_query_exporter",
		Short: "MongoDB aggregation exporter for prometheus",
		Long:  `Export different aggregations from MongoDB as prometheus comptatible metrics.`,
		Run: func(cmd *cobra.Command, args []string) {
			var config collector.Config

			err := viper.Unmarshal(&config)
			if err != nil {
				panic(err)
			}

			level, err := log.ParseLevel(logLevel)
			if err != nil {
				panic(err)
			}

			log.SetLevel(level)
			collector.Run(&config)
		},
	}
)

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "config file (default is $HOME/.mongodb_query_exporter/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Define a log level (default is info)")
	rootCmd.PersistentFlags().StringVarP(&bind, "bind", "b", ":9412", "config file (default is :9412)")
	rootCmd.PersistentFlags().StringVarP(&uri, "uri", "u", "mongodb://localhost:27017", "MongoDB URI (default is mongodb://localhost:27017)")
	rootCmd.PersistentFlags().IntVarP(&timeout, "timeout", "t", 10, "MongoDB connection timeout (default is 10 secconds")
	viper.BindPFlag("logLevel", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("bind", rootCmd.PersistentFlags().Lookup("bind"))
	viper.BindPFlag("mongodb.uri", rootCmd.PersistentFlags().Lookup("uri"))
	viper.BindPFlag("mongodb.connection_timeout", rootCmd.PersistentFlags().Lookup("timeout"))
}

func initConfig() {
	if configPath != "" {
		// Use config file from the flag.
		viper.SetConfigFile(configPath)
	} else {
		// Find home directory.
		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
		}

		// Search config in home directory with name ".mongodb_query_exporter" (without extension).
		viper.AddConfigPath(usr.HomeDir + "/.mongodb_query_exporter")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Errorf("failed to open config file %s", err)
	} else {
		log.Infof("using config file %s", viper.ConfigFileUsed())
	}
}
