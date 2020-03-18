package collector

import (
	"context"
	"fmt"
	"net/http"
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
	DefaultInterval   int64
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
	Interval    int64
	ConstLabels prometheus.Labels
	Mode        string
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

func (collector *Collector) initializeMetric(metric *Metric) error {
	// Set cache time (pull interval)
	if metric.Interval > 0 {
		metric.sleep = time.Duration(metric.Interval) * time.Second
	} else if collector.Config.MongoDBConfig.DefaultInterval > 0 {
		metric.sleep = time.Duration(collector.Config.MongoDBConfig.DefaultInterval) * time.Second
		metric.Interval = collector.Config.MongoDBConfig.DefaultInterval
	} else {
		metric.sleep = 5 * time.Second
		metric.Interval = 5
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

	return nil
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

	ctx, cancel := context.WithTimeout(context.Background(), collector.Config.MongoDBConfig.ConnectionTimeout*time.Second)
	defer cancel()

	cursor, err := collector.Driver.Aggregate(ctx, metric.Database, metric.Collection, pipeline)

	if err != nil {
		return err
	}

	var multierr *multierror.Error
	var i int

	for cursor.Next(ctx) {
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

var cursors = make(map[string][]string)

func (collector *Collector) PushHandler(metric *Metric) {
	//start only one changestream per database/collection
	if val, ok := cursors[metric.Database]; ok {
		for _, coll := range val {
			if coll == metric.Collection {
				return
			}
		}

		cursors[metric.Database] = append(cursors[metric.Database], metric.Collection)
	} else {
		cursors[metric.Database] = []string{metric.Collection}
	}

	err := collector.pushUpdate(metric)
	if err != nil {
		log.Errorf("failed to handle realtime updates for %s, error %s", metric.Name, err)
	}
}

// Run metric collector for each metric either in push or pull mode
func (collector *Collector) Run() {
	for _, metric := range collector.Config.Metrics {
		go func(metric *Metric) {
			log.Infof("initialize metric %s", metric.Name)

			err := collector.initializeMetric(metric)

			if err != nil {
				log.Errorf("failed to initialize metric %s with error %s", metric.Name, err)
				return
			}

			//If the metric is realtime we start a mongodb changestream and wait for changes instead pull (interval)
			if metric.Mode == "" || metric.Mode == "pull" {
				collector.PullHandler(metric)
			} else if metric.Mode == "push" {
				collector.PushHandler(metric)
			}
		}(metric)
	}
}

//Execute aggregations and update metrics in intervals
func (collector *Collector) PullHandler(metric *Metric) {
	for {
		err := collector.updateMetric(metric)

		if err != nil {
			log.Errorf("failed to handle metric %s, awaiting the next pull. failed with error %s", err, metric.Name)
		}

		log.Infof("wait %ds to refresh metric %s", metric.Interval, metric.Name)
		time.Sleep(metric.sleep)
	}
}

func (collector *Collector) pushUpdate(metric *Metric) error {
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
			if metric.Mode == "push" && metric.Database == result.NS.DB && metric.Collection == result.NS.Coll && metric.metric != nil {
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
