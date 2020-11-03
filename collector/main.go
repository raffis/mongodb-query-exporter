package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	ctx context.Context
)

// A collector is a metric collector group for one single MongoDB server.
// Each collector needs a MongoDB client and a list of metrics which should be generated.
// You may initialize multiple collectors for multiple MongoDB servers.
type Collector struct {
	servers []*server
	logger  Logger
	config  *Config
	metrics []*Metric
	counter *prometheus.CounterVec
	cache   map[string]*cacheEntry
}

type cacheEntry struct {
	m   prometheus.Metric
	ttl int64
}

type server struct {
	name   string
	driver Driver
}

type option func(c *Collector)

// Create a new collector
func New(opts ...option) *Collector {
	c := &Collector{
		logger: &dummyLogger{},
		config: &Config{
			QueryTimeout: 10,
		},
	}

	c.cache = make(map[string]*cacheEntry)

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Pass a counter metrics about query stats
func WithCounter(m *prometheus.CounterVec) option {
	return func(c *Collector) {
		c.counter = m
	}
}

// Pass a logger to the collector
func WithLogger(l Logger) option {
	return func(c *Collector) {
		c.logger = l
	}
}

// Pass a collector configuration (Defaults for metrics)
func WithConfig(conf *Config) option {
	return func(c *Collector) {
		c.config = conf
	}
}

// Collector configuration with default metric configurations
type Config struct {
	QueryTimeout      time.Duration
	DefaultCache      int64
	DefaultMode       string
	DefaultDatabase   string
	DefaultCollection string
}

// A metric defines what metric should be generated from what MongoDB aggregation.
// The pipeline configures (as JSON) the aggreation query
type Metric struct {
	Name        string
	Type        string
	Servers     []string
	Help        string
	Value       string
	Cache       int64
	ConstLabels prometheus.Labels
	Mode        string
	Labels      []string
	Database    string
	Collection  string
	Pipeline    string
	desc        *prometheus.Desc
	pipeline    bson.A
	validUntil  time.Time
}

var (
	//Only Gauge is a supported metric types
	ErrInvalidType = errors.New("unknown metric type provided. Only gauge is supported")
	//Only Gauge and Counter are supported metric types
	ErrServerNotRegistered = errors.New("server needs to be registered")
	//The value was not found in the aggregation result set
	ErrValueNotFound = errors.New("value not found in result set")
	//No cached metric available
	ErrNotCached = errors.New("metric not available from cache")
)

const (
	//Gauge metric type (Can increase and decrease)
	TypeGauge = "gauge"
	//Pull mode (with interval)
	ModePull = "pull"
	//Push mode (Uses changestream which is only supported with MongoDB >= 3.6)
	ModePush = "push"
)

func (c *Collector) generateMetrics(metric *Metric, srv *server, ch chan<- prometheus.Metric) error {
	c.logger.Debugf("generate metric %s from server %s", metric.Name, srv.name)

	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeout*time.Second)
	defer cancel()

	cursor, err := srv.driver.Aggregate(ctx, metric.Database, metric.Collection, metric.pipeline)

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

		m, err := createMetric(metric, result)

		if err != nil {
			return err
		}

		c.updateCache(metric, srv, m)
		ch <- m
	}

	if i == 0 {
		return fmt.Errorf("metric %s aggregation returned an emtpy result set", metric.Name)
	}

	return multierr.ErrorOrNil()
}

func createMetric(metric *Metric, result AggregationResult) (prometheus.Metric, error) {
	value, err := metric.getValue(result)
	if err != nil {
		return nil, err
	}

	labels, err := metric.getLabels(result)
	if err != nil {
		return nil, err
	}

	return prometheus.NewConstMetric(metric.desc, prometheus.GaugeValue, value, labels...)
}

func (metric *Metric) getValue(result AggregationResult) (float64, error) {
	if val, ok := result[metric.Value]; ok {
		switch val.(type) {
		case float32:
			value := float64(val.(float32))
			return value, nil
		case float64:
			value := val.(float64)
			return value, nil
		case int32:
			value := float64(val.(int32))
			return value, nil
		case int64:
			value := float64(val.(int64))
			return value, nil
		default:
			return 0, fmt.Errorf("provided value taken from the aggregation result has to be a number, type %T given", val)
		}
	}

	return 0, ErrValueNotFound
}

func (metric *Metric) getLabels(result AggregationResult) ([]string, error) {
	var labels []string

	for _, label := range metric.Labels {
		if val, ok := result[label]; ok {
			switch val.(type) {
			case string:
				labels = append(labels, val.(string))
			default:
				return labels, fmt.Errorf("provided label value taken from the aggregation result has to be a string, type %T given", val)
			}
		} else {
			return labels, fmt.Errorf("required label %s not found in result set", label)
		}
	}

	return labels, nil
}

// Run metric c for each metric either in push or pull mode
func (c *Collector) RegisterServer(name string, driver Driver) error {
	for _, srv := range c.servers {
		if srv.name == name {
			return fmt.Errorf("server %s is already registered", name)
		}
	}

	srv := &server{
		name:   name,
		driver: driver,
	}

	c.servers = append(c.servers, srv)
	return nil
}

// Run metric c for each metric either in push or pull mode
func (c *Collector) RegisterMetric(metric *Metric) error {
	desc := prometheus.NewDesc(
		metric.Name,
		metric.Help,
		metric.Labels,
		metric.ConstLabels,
	)
	metric.desc = desc

	if len(metric.Servers) != 0 && len(metric.Servers) != len(c.GetServers(metric.Servers)) {
		return ErrServerNotRegistered
	}

	err := bson.UnmarshalExtJSON([]byte(metric.Pipeline), false, &metric.pipeline)
	if err != nil {
		return errors.Wrap(err, "failed to decode json aggregation pipeline")
	}

	if c.config.DefaultCache > 0 && metric.Cache != 0 {
		metric.Cache = c.config.DefaultCache
	}

	c.metrics = append(c.metrics, metric)
	return nil
}

// Return registered drivers
// You may provide a list of names to only return matching drivers by name
func (c *Collector) GetServers(names []string) []*server {
	var servers []*server
	for _, srv := range c.servers {
		//if we have no filter given just add all drivers to be returned
		if len(names) == 0 {
			servers = append(servers, srv)
			continue
		}

		for _, name := range names {
			if srv.name == name {
				servers = append(servers, srv)
			}
		}
	}

	return servers
}

// Describe is implemented with DescribeByCollect
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	if c.counter != nil {
		c.counter.Describe(ch)
	}

	for _, metric := range c.metrics {
		ch <- metric.desc
	}
}

// Collect all metrics from queries
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.logger.Debugf("start collecting metrics")
	var wg sync.WaitGroup
	for _, metric := range c.metrics {
		for _, srv := range c.GetServers(metric.Servers) {
			m, err := c.getCached(metric, srv)

			if err == nil {
				c.logger.Debugf("use value from cache for %s", metric.Name)
				ch <- m
				continue
			}

			wg.Add(1)
			go func(metric *Metric, srv *server, ch chan<- prometheus.Metric) {
				defer wg.Done()
				err := c.generateMetrics(metric, srv, ch)

				if c.counter == nil {
					return
				}

				var result string
				if err == nil {
					result = "SUCCESS"
				} else {
					c.logger.Errorf("failed to generate metric : %s", err)

					result = "ERROR"
				}

				c.counter.With(prometheus.Labels{
					"server": srv.name,
					"metric": metric.Name,
					"result": result,
				}).Inc()

				c.counter.Collect(ch)
			}(metric, srv, ch)
		}
	}

	wg.Wait()
}

func (c *Collector) updateCache(metric *Metric, srv *server, m prometheus.Metric) {
	if (metric.Mode == ModePush && metric.Cache == 0) || metric.Cache == -1 {
		c.logger.Debugf("cache metric %s until new push", metric.Name)
		c.cache[metric.Name+srv.name] = &cacheEntry{m, -1}
	} else if metric.Cache > 0 {
		c.logger.Debugf("cache metric %s for %d", metric.Name, metric.Cache)
		c.cache[metric.Name+srv.name] = &cacheEntry{m, time.Now().Unix() + metric.Cache}
	} else {
		c.logger.Debugf("skip cache for %s", metric.Name)
	}
}

func (c *Collector) getCached(metric *Metric, srv *server) (prometheus.Metric, error) {
	if e, exists := c.cache[metric.Name+srv.name]; exists {
		if e.ttl == -1 || e.ttl >= time.Now().Unix() {
			return e.m, nil
		}

		// entry can be removed from cache since its expired
		delete(c.cache, metric.Name+srv.name)
	}

	return nil, ErrNotCached
}

// Start MongoDB watchers for metrics where push is enabled.
// As soon as a new event is registered the cache gets invalidated and the aggregation
// will be re evaluated during the next scrape.
// This is a non blocking operation.
func (c *Collector) StartCacheInvalidator() error {
	for _, metric := range c.metrics {
		for _, srv := range c.GetServers(metric.Servers) {
			go func(metric *Metric, srv *server) {
				err := c.pushUpdate(metric, srv)

				if err != nil {
					c.logger.Errorf("%s; failed to watch for updates, fallback to pull", err)
				}
			}(metric, srv)
		}
	}

	return nil
}

func (c *Collector) pushUpdate(metric *Metric, srv *server) error {
	c.logger.Infof("start changestream on %s.%s, waiting for changes", metric.Database, metric.Collection)
	cursor, err := srv.driver.Watch(ctx, metric.Database, metric.Collection, metric.pipeline)

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

		//Invalidate cached entry, aggregation must be executed during the next scrape
		delete(c.cache, metric.Name+srv.name)
	}

	return nil
}
