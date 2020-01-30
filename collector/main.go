package collector

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ctx    context.Context
	client *mongo.Client
)

type MongoDBConfig struct {
	URI               string
	MaxConnections    int32
	ConnectionTimeout time.Duration
}

type Config struct {
	MongoDBConfig MongoDBConfig `mapstructure:"mongodb"`
	MetricOptions MetricOptions
	Bind          string
	LogLevel      string
	Metrics       []*Metric
}

type MetricOptions struct {
	DefaultCacheTime  int64
	DefaultDatabase   string
	DefaultCollection string
}

type Metric struct {
	Name        string
	Type        string
	Help        string
	Value       string
	CacheTime   int64
	ConstLabels prometheus.Labels
	Realtime    bool
	Labels      []string
	Database    string
	Collection  string
	Pipeline    string
	metric      interface{}
	sleep       time.Duration
	value       *float64
}

type ChangeStreamEventNamespace struct {
	DB   string
	Coll string
}

type ChangeStreamEvent struct {
	NS *ChangeStreamEventNamespace
}

type AggregationResult map[string]interface{}

const (
	typeGauge   = "gauge"
	typeCounter = "counter"
)

func (config *Config) initializeMetrics() {
	if len(config.Metrics) == 0 {
		log.Warning("no metrics have been configured")
		return
	}

	for _, metric := range config.Metrics {
		log.Infof("initialize metric %s", metric.Name)

		//set cache time (pull interval)
		if metric.CacheTime > 0 {
			metric.sleep = time.Duration(metric.CacheTime) * time.Second
		} else if config.MetricOptions.DefaultCacheTime > 0 {
			metric.sleep = time.Duration(config.MetricOptions.DefaultCacheTime) * time.Second
		} else {
			metric.sleep = 5 * time.Second
		}

		//initialize prometheus metric
		var err error
		if len(metric.Labels) == 0 {
			err = metric.initializeUnlabeledMetric()
		} else {
			err = metric.initializeLabeledMetric()
		}

		//fetch initial value
		if err != nil {
			log.Errorf("failed to initialize metric %s with error %s", metric.Name, err)
		} else {
			go func(metric *Metric) {
				err := metric.update()

				if err != nil {
					log.Errorf("failed to fetch initial value for %s with error %s", metric.Name, err)
				}
			}(metric)
		}
	}
}

func (metric *Metric) getOptions() interface{} {
	switch metric.Type {
	case typeGauge:
		return prometheus.GaugeOpts{
			Name:        metric.Name,
			Help:        metric.Help,
			ConstLabels: metric.ConstLabels,
		}
	case typeCounter:
		return prometheus.CounterOpts{
			Name:        metric.Name,
			Help:        metric.Help,
			ConstLabels: metric.ConstLabels,
		}
	default:
		return errors.New("unknown metric type provided. Only [gauge,conuter] are valid options")
	}
}

func (metric *Metric) initializeLabeledMetric() error {
	switch metric.Type {
	case typeGauge:
		metric.metric = promauto.NewGaugeVec(metric.getOptions().(prometheus.GaugeOpts), metric.Labels)
	case typeCounter:
		metric.metric = promauto.NewCounterVec(metric.getOptions().(prometheus.CounterOpts), metric.Labels)
	default:
		return errors.New("unknown metric type provided. Only [gauge,counter] are valid options")
	}

	return nil
}

func (metric *Metric) initializeUnlabeledMetric() error {
	switch metric.Type {
	case typeGauge:
		metric.metric = promauto.NewGauge(metric.getOptions().(prometheus.GaugeOpts))
	case typeCounter:
		metric.metric = promauto.NewCounter(metric.getOptions().(prometheus.CounterOpts))
	default:
		return errors.New("unknown metric type provided. Only [gauge,counter] are valid options")
	}

	return nil
}

func (config *Config) startListeners() {
	for _, metric := range config.Metrics {
		//If the metric is realtime we start a mongodb changestream and wait for changes instead pull (interval)
		if metric.Realtime == true {
			continue
		}

		//do not start listeneres for uninitialized metrics due errors
		if metric.metric == nil {
			continue
		}

		go func(metric *Metric) {
			for {
				err := metric.update()

				if err != nil {
					log.Errorf("failed to handle metric %s, abort listen on metric %s", err, metric.Name)
					return
				}

				log.Debugf("wait %ds to refresh metric %s", metric.CacheTime, metric.Name)
				time.Sleep(metric.sleep)
			}
		}(metric)
	}
}

func (metric *Metric) update() error {
	var pipeline bson.A
	log.Debugf("aggregate mongodb pipeline %s", metric.Pipeline)
	err := bson.UnmarshalExtJSON([]byte(metric.Pipeline), false, &pipeline)

	if err != nil {
		return err
	}

	cursor, err := client.Database(metric.Database).Collection(metric.Collection).Aggregate(
		context.Background(),
		pipeline,
	)

	if err != nil {
		return err
	}

	for cursor.Next(context.TODO()) {
		var result AggregationResult

		err := cursor.Decode(&result)
		log.Debugf("found record %s from metric %s", result, metric.Name)

		if err != nil {
			log.Errorf("failed decode record %s", err)
			continue
		}

		err = metric.updateValue(result)
		if err != nil {
			log.Errorf("failed update record %s", err)
		}
	}

	return nil
}

func (metric *Metric) updateValue(result AggregationResult) error {
	if len(metric.Labels) == 0 {
		return metric.updateUnlabeled(result)
	}

	return metric.updateLabeled(result)
}

func (metric *Metric) updateUnlabeled(result AggregationResult) error {
	value, err := metric.getValue(result)
	if err != nil {
		return err
	}

	switch metric.Type {
	case typeGauge:
		metric.metric.(prometheus.Gauge).Set(*value)
	case typeCounter:
		new := *value - *metric.value
		metric.metric.(prometheus.Counter).Add(new)
		metric.value = &new
	}

	return nil
}

func (metric *Metric) updateLabeled(result AggregationResult) error {
	value, err := metric.getValue(result)
	if err != nil {
		return err
	}

	labels, err := metric.getLabels(result)
	if err != nil {
		return err
	}

	switch metric.Type {
	case typeGauge:
		metric.metric.(*prometheus.GaugeVec).With(labels).Set(*value)
	case typeCounter:
		var new float64
		if metric.value == nil {
			new = *value
		} else {
			new = *value - *metric.value
		}

		metric.value = &new
		if new <= *metric.value {
			return fmt.Errorf("failed to increase counter for %s, counter can not be decreased", metric.Name)
		}

		metric.metric.(*prometheus.CounterVec).With(labels).Add(new)
	}

	return nil
}

func (metric *Metric) getValue(result AggregationResult) (*float64, error) {
	if val, ok := result[metric.Value]; ok {
		switch val.(type) {
		case int32:
			value := float64(val.(int32))
			return &value, nil
		case int64:
			value := float64(val.(int64))
			return &value, nil
		default:
			return nil, fmt.Errorf("provided value taken from the aggregation result has to be a number, type %T given", val)
		}
	}

	return nil, errors.New("value not found in result set")
}

func (metric *Metric) getLabels(result AggregationResult) (prometheus.Labels, error) {
	var labels = make(prometheus.Labels)

	for _, label := range metric.Labels {
		if val, ok := result[label]; ok {
			switch val.(type) {
			case string:
				labels[label] = val.(string)
			default:
				return nil, fmt.Errorf("provided label value taken from the aggregation result has to be a string, type %T given", val)
			}
		} else {
			return nil, fmt.Errorf("required label %s not found in result set", label)
		}
	}

	return labels, nil
}

func (config *Config) realtimeListener() error {
	var cursors = make(map[string][]string)

METRICS:
	for _, metric := range config.Metrics {
		if metric.Realtime == false {
			continue
		}

		//start only one changestream per database/collection
		if val, ok := cursors[metric.Database]; ok {
			for _, coll := range val {
				if coll == metric.Collection {
					continue METRICS
				}
			}

			cursors[metric.Database] = append(cursors[metric.Database], metric.Collection)
		} else {
			cursors[metric.Database] = []string{metric.Collection}
		}

		//start changestream for each database/collection
		go func(metric *Metric) {
			log.Infof("start changestream on %s.%s, waiting for changes", metric.Database, metric.Collection)
			cursor, err := client.Database(metric.Database).Collection(metric.Collection).Watch(ctx, mongo.Pipeline{})

			if err != nil {
				log.Errorf("failed to start changestream listener %s", err)
				return
			}

			defer cursor.Close(ctx)

			for cursor.Next(context.TODO()) {
				var result ChangeStreamEvent

				err := cursor.Decode(&result)

				if err != nil {
					log.Errorf("failed decode record %s", err)
					continue
				}

				log.Debugf("found new changestream event in %s.%s", metric.Database, metric.Collection)

				for _, metric := range config.Metrics {
					if metric.Realtime == true && metric.Database == result.NS.DB && metric.Collection == result.NS.Coll {
						err := metric.update()

						if err != nil {
							log.Errorf("failed to update metric %s, failed with error %s", metric.Name, err)
						}
					}
				}
			}
		}(metric)
	}

	return nil
}

// Run executes a blocking http server. Starts the http listener with the /metrics endpoint
// and parses all configured metrics passed by config
func Run(config *Config) {
	ctx, cancel := context.WithTimeout(context.Background(), config.MongoDBConfig.ConnectionTimeout*time.Second)
	defer cancel()

	if config.MongoDBConfig.URI == "" {
		config.MongoDBConfig.URI = "mongodb://localhost:27017"
	}

	log.Printf("connect to mongodb using uri %s, connect_timeout=%d", config.MongoDBConfig.URI, config.MongoDBConfig.ConnectionTimeout)
	var err error
	client, err = mongo.Connect(ctx, options.Client().ApplyURI(config.MongoDBConfig.URI))
	if err != nil {
		panic(err)
	}

	// Check the connection, terminate if MongoDB is not reachable
	log.Debugf("ping mongodb and enforce connection")
	err = client.Ping(ctx, nil)
	if err != nil {
		panic(err)
	}

	log.Debugf("mongodb up an reachable, start listeners")
	config.initializeMetrics()
	config.startListeners()
	config.realtimeListener()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) })
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("handle incoming http request /metrics")

		// Check the connection, if not reachable return an error 500
		ctx, cancel := context.WithTimeout(context.Background(), config.MongoDBConfig.ConnectionTimeout*time.Second)
		defer cancel()

		err = client.Ping(ctx, nil)
		if err != nil {
			log.Errorf("mongodb not reachable (ping) return 500 Internal Server Error, %s", err)
			w.WriteHeader(500)
		} else {
			promhttp.Handler().ServeHTTP(w, r)
		}
	})

	if config.Bind == "" {
		config.Bind = ":9412"
	}

	log.Printf("start http listener on %s", config.Bind)
	err = http.ListenAndServe(config.Bind, nil)

	//if the port is already in use or another fatal error panic
	if err != nil {
		panic(err)
	}
}
