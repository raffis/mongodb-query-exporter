package collector

import (
	"context"
	"fmt"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	ctx context.Context
)

// A collector is a metric collector group for one single MongoDB server.
// Each collector needs a MongoDB client and a list of metrics which should be generated.
// You may initialize multiple collectors for multiple MongoDB servers.
type collector struct {
	driver  Driver
	logger  Logger
	config  *Config
	metrics []*Metric
}

type option func(c *collector)

// Create a new collector
func New(opts ...option) *collector {
	c := &collector{
		logger: &dummyLogger{},
		config: &Config{
			QueryTimeout:    10,
			DefaultInterval: 10,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Pass a logger to the collector
func WithLogger(l Logger) option {
	return func(c *collector) {
		c.logger = l
	}
}

// Pass a collector configuration (Defaults for metrics)
func WithConfig(conf *Config) option {
	return func(c *collector) {
		c.config = conf
	}
}

// Pass a MongoDB client instance
func WithDriver(d Driver) option {
	return func(c *collector) {
		c.driver = d
	}
}

// collector configuration with default metric configurations
type Config struct {
	QueryTimeout      time.Duration
	DefaultInterval   int64
	DefaultDatabase   string
	DefaultCollection string
}

// A metric defines what metric should be generated from what MongoDB aggregation.
// The pipeline configures (as JSON) the aggreation query
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

var (
	//Only Gauge and Counter are supported metric types
	ErrInvalidType = errors.New("unknown metric type provided. Only [gauge,counter] are valid options")
)

const (
	//Gauge metric type (Can increase and decrease)
	TypeGauge = "gauge"
	//Counter metric type (increased number)
	TypeCounter = "counter"
	//Pull mode (with interval)
	ModePull = "pull"
	//Push mode (Uses changestream which is only supported with MongoDB >= 3.6)
	ModePush = "push"
)

func (c *collector) initializeMetric(metric *Metric) error {
	// Set cache time (pull interval)
	if metric.Interval > 0 {
		metric.sleep = time.Duration(metric.Interval) * time.Second
	} else if c.config.DefaultInterval > 0 {
		metric.sleep = time.Duration(c.config.DefaultInterval) * time.Second
		metric.Interval = c.config.DefaultInterval
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
	case TypeGauge:
		return prometheus.GaugeOpts{
			Name:        metric.Name,
			Help:        metric.Help,
			ConstLabels: metric.ConstLabels,
		}
	case TypeCounter:
		return prometheus.CounterOpts{
			Name:        metric.Name,
			Help:        metric.Help,
			ConstLabels: metric.ConstLabels,
		}
	default:
		return ErrInvalidType
	}
}

func (metric *Metric) initializeLabeledMetric() error {
	switch metric.Type {
	case TypeGauge:
		metric.metric = promauto.NewGaugeVec(metric.getOptions().(prometheus.GaugeOpts), metric.Labels)
	case TypeCounter:
		metric.metric = promauto.NewCounterVec(metric.getOptions().(prometheus.CounterOpts), metric.Labels)
	default:
		return ErrInvalidType
	}

	return nil
}

func (metric *Metric) initializeUnlabeledMetric() error {
	switch metric.Type {
	case TypeGauge:
		metric.metric = promauto.NewGauge(metric.getOptions().(prometheus.GaugeOpts))
	case TypeCounter:
		metric.metric = promauto.NewCounter(metric.getOptions().(prometheus.CounterOpts))
	default:
		return ErrInvalidType
	}

	return nil
}

func (c *collector) updateMetric(metric *Metric) error {
	var pipeline bson.A
	c.logger.Debugf("aggregate mongodb pipeline %s", metric.Pipeline)
	err := bson.UnmarshalExtJSON([]byte(metric.Pipeline), false, &pipeline)

	if err != nil {
		return errors.Wrap(err, "failed to decode json aggregation pipeline")
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeout*time.Second)
	defer cancel()

	cursor, err := c.driver.Aggregate(ctx, metric.Database, metric.Collection, pipeline)

	if err != nil {
		return err
	}

	var multierr *multierror.Error
	var i int

	for cursor.Next(ctx) {
		i++
		var result AggregationResult

		err := cursor.Decode(&result)
		c.logger.Debugf("found record %s from metric %s", result, metric.Name)

		if err != nil {
			multierr = multierror.Append(multierr, err)
			c.logger.Errorf("failed decode record %s", err)
			continue
		}

		err = metric.updateValue(result)
		if err != nil {
			multierr = multierror.Append(multierr, err)
			c.logger.Errorf("failed update record %s", err)
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
	case TypeGauge:
		metric.metric.(prometheus.Gauge).Set(*value)
	case TypeCounter:
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
	case TypeGauge:
		metric.metric.(*prometheus.GaugeVec).With(labels).Set(*value)
	case TypeCounter:
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

func (c *collector) addPushHandler(metric *Metric) {
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

	err := c.pushUpdate(metric)
	if err != nil {
		c.logger.Errorf("failed to handle realtime updates for %s, error %s", metric.Name, err)
	}
}

// Run metric c for each metric either in push or pull mode
func (c *collector) WithMetric(metric *Metric) {
	c.logger.Infof("initialize metric %s", metric.Name)
	err := c.initializeMetric(metric)

	if err != nil {
		c.logger.Errorf("failed to initialize metric %s with error %s", metric.Name, err)
		return
	}

	//If the metric is realtime we start a mongodb changestream and wait for changes instead pull (interval)
	if metric.Mode == "" || metric.Mode == ModePull {
		c.addPullHandler(metric)
	} else if metric.Mode == ModePush {
		c.addPushHandler(metric)
	}
}

//Execute aggregations and update metrics in intervals
func (c *collector) addPullHandler(metric *Metric) {
	for {
		err := c.updateMetric(metric)

		if err != nil {
			c.logger.Errorf("failed to handle metric %s, awaiting the next pull. failed with error %s", metric.Name, err)
		}

		c.logger.Infof("wait %ds to refresh metric %s", metric.Interval, metric.Name)
		time.Sleep(metric.sleep)
	}
}

func (c *collector) pushUpdate(metric *Metric) error {
	c.logger.Infof("start changestream on %s.%s, waiting for changes", metric.Database, metric.Collection)
	cursor, err := c.driver.Watch(ctx, metric.Database, metric.Collection, mongo.Pipeline{})

	if err != nil {
		return fmt.Errorf("failed to start changestream listener %s", err)
	}

	defer cursor.Close(ctx)

	for cursor.Next(context.TODO()) {
		var result ChangeStreamEvent

		err := cursor.Decode(&result)
		if err != nil {
			c.logger.Errorf("failed decode record %s", err)
			continue
		}

		c.logger.Debugf("found new changestream event in %s.%s", metric.Database, metric.Collection)

		var errors *multierror.Error

		for _, metric := range c.metrics {
			if metric.Mode == "push" && metric.Database == result.NS.DB && metric.Collection == result.NS.Coll && metric.metric != nil {
				err := c.updateMetric(metric)

				if err != nil {
					errors = multierror.Append(errors, fmt.Errorf("failed to update metric %s, failed with error %s", metric.Name, err))
					c.logger.Errorf("failed to update metric %s, failed with error %s", metric.Name, err)
				}
			}
		}

		return errors.ErrorOrNil()
	}

	return nil
}
