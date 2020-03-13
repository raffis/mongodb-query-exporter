package collector

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ctx context.Context
)

// MongoDB client options
type MongoDBConfig struct {
	URI               string
	MaxConnections    int32
	ConnectionTimeout time.Duration
	DefaultCacheTime  int64
	DefaultDatabase   string
	DefaultCollection string
}

// The collector holds the configureation and the mongodb driver
type Collector struct {
	Driver Driver
	Config *Config
}

// Global configuration which holds all monitored aggregations
type Config struct {
	MongoDBConfig MongoDBConfig `mapstructure:"mongodb"`
	Bind          string
	LogLevel      string
	Metrics       []*Metric
}

// MongoDB aggregation metric
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

const (
	typeGauge   = "gauge"
	typeCounter = "counter"
)

func (collector *Collector) initializeMetrics() {
	var wg sync.WaitGroup

	if len(collector.Config.Metrics) == 0 {
		log.Warning("no metrics have been configured")
		return
	}

	for _, metric := range collector.Config.Metrics {
		log.Infof("initialize metric %s", metric.Name)
		wg.Add(1)

		go func(metric *Metric) {
			defer wg.Done()

			err := collector.initializeMetric(metric)

			if err != nil {
				log.Errorf("failed to initialize metric %s with error %s", metric.Name, err)
			}
		}(metric)
	}

	wg.Wait()
}

func (collector *Collector) initializeMetric(metric *Metric) error {
	log.Infof("initialize metric %s", metric.Name)

	// Set cache time (pull interval)
	if metric.CacheTime > 0 {
		metric.sleep = time.Duration(metric.CacheTime) * time.Second
	} else if collector.Config.MongoDBConfig.DefaultCacheTime > 0 {
		metric.sleep = time.Duration(collector.Config.MongoDBConfig.DefaultCacheTime) * time.Second
		metric.CacheTime = collector.Config.MongoDBConfig.DefaultCacheTime
	} else {
		metric.sleep = 5 * time.Second
		metric.CacheTime = 5
	}

	// Initialize prometheus metric
	var err error
	if len(metric.Labels) == 0 {
		err = metric.initializeUnlabeledMetric()
	} else {
		err = metric.initializeLabeledMetric()
	}

	if err != nil {
		return fmt.Errorf("failed to initialize metric %s with error %s", metric.Name, err)
	}

	return collector.updateMetric(metric)
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

func (collector *Collector) updateMetric(metric *Metric) error {
	var pipeline bson.A
	log.Debugf("aggregate mongodb pipeline %s", metric.Pipeline)
	err := bson.UnmarshalExtJSON([]byte(metric.Pipeline), false, &pipeline)

	if err != nil {
		return errors.Wrap(err, "failed to decode json aggregation pipeline")
	}

	cursor, err := collector.Driver.Aggregate(context.Background(), metric.Database, metric.Collection, pipeline)

	if err != nil {
		return err
	}

	var multierr *multierror.Error
	var i int

	for cursor.Next(context.TODO()) {
		i++
		var result AggregationResult

		err := cursor.Decode(&result)
		log.Debugf("found record %s from metric %s", result, metric.Name)

		if err != nil {
			multierr = multierror.Append(multierr, err)
			log.Errorf("failed decode record %s", err)
			continue
		}

		err = metric.updateValue(result)
		if err != nil {
			multierr = multierror.Append(multierr, err)
			log.Errorf("failed update record %s", err)
		}
	}

	if i == 0 {
		return fmt.Errorf("metric %s aggregation returned an emtpy result set", metric.Name)
	}

	return multierr.ErrorOrNil()
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
		err = metric.increaseCounterValue(value)
		if err != nil {
			return err
		}

		metric.metric.(prometheus.Counter).Add(*metric.value)
	}

	return nil
}

func (metric *Metric) increaseCounterValue(value *float64) error {
	var new float64
	if metric.value == nil {
		new = *value
	} else {
		new = *value - *metric.value
	}

	if metric.value != nil && new <= *metric.value {
		return fmt.Errorf("failed to increase counter for %s, counter can not be decreased", metric.Name)
	}

	metric.value = &new
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
		err = metric.increaseCounterValue(value)
		if err != nil {
			return err
		}

		metric.metric.(*prometheus.CounterVec).With(labels).Add(*metric.value)
	}

	return nil
}

func (metric *Metric) getValue(result AggregationResult) (*float64, error) {
	if val, ok := result[metric.Value]; ok {
		switch val.(type) {
		case float32:
			value := float64(val.(float32))
			return &value, nil
		case float64:
			value := val.(float64)
			return &value, nil
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

func (collector *Collector) startPushListeners() error {
	var cursors = make(map[string][]string)

METRICS:
	for _, metric := range collector.Config.Metrics {
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
			err := collector.realtimeUpdate(metric)

			if err != nil {
				log.Errorf("failed to handle realtime updates for %s, error %s", metric.Name, err)
			}
		}(metric)
	}

	return nil
}

func (collector *Collector) startPullListeners() {
	for _, metric := range collector.Config.Metrics {
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
				err := collector.updateMetric(metric)

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

func (collector *Collector) realtimeUpdate(metric *Metric) error {
	log.Infof("start changestream on %s.%s, waiting for changes", metric.Database, metric.Collection)
	cursor, err := collector.Driver.Watch(ctx, metric.Database, metric.Collection, mongo.Pipeline{})

	if err != nil {
		return fmt.Errorf("failed to start changestream listener %s", err)
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

		var errors *multierror.Error

		for _, metric := range collector.Config.Metrics {
			if metric.Realtime == true && metric.Database == result.NS.DB && metric.Collection == result.NS.Coll {
				err := collector.updateMetric(metric)

				if err != nil {
					errors = multierror.Append(errors, fmt.Errorf("failed to update metric %s, failed with error %s", metric.Name, err))
					log.Errorf("failed to update metric %s, failed with error %s", metric.Name, err)
				}
			}
		}

		return errors.ErrorOrNil()
	}

	return nil
}

// Initialize metrics and start listeners
func (collector *Collector) Run() {
	collector.initializeMetrics()
	collector.startPullListeners()
	collector.startPushListeners()
}

// Run executes a blocking http server. Starts the http listener with the /metrics endpoint
// and parses all configured metrics passed by config
func RunAndBind(config *Config) {
	ctx, cancel := context.WithTimeout(context.Background(), config.MongoDBConfig.ConnectionTimeout*time.Second)
	defer cancel()

	if config.MongoDBConfig.URI == "" {
		config.MongoDBConfig.URI = "mongodb://localhost:27017"
	}

	log.Printf("connect to mongodb, connect_timeout=%d", config.MongoDBConfig.ConnectionTimeout)
	var err error

	collector := &Collector{}
	collector.Driver = &MongoDBDriver{}
	err = collector.Driver.Connect(ctx, options.Client().ApplyURI(config.MongoDBConfig.URI))
	collector.Config = config

	if err != nil {
		panic(err)
	}

	// Check the connection, terminate if MongoDB is not reachable
	log.Debugf("ping mongodb and enforce connection")
	err = collector.Driver.Ping(ctx, nil)
	if err != nil {
		panic(err)
	}

	log.Debugf("mongodb up an reachable, start listeners")
	collector.Run()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "OK", http.StatusOK) })
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("handle incoming http request /metrics")

		// Check the connection, if not reachable return an error 500
		ctx, cancel := context.WithTimeout(context.Background(), config.MongoDBConfig.ConnectionTimeout*time.Second)
		defer cancel()

		err = collector.Driver.Ping(ctx, nil)
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

	// If the port is already in use or another fatal error panic
	if err != nil {
		panic(err)
	}
}
