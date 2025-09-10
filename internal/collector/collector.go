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

// A collector is a metric collector group for one single MongoDB server.
// Each collector needs a MongoDB client and a list of metrics which should be generated.
// You may initialize multiple collectors for multiple MongoDB servers.
type Collector struct {
	servers      []*server
	logger       Logger
	config       *Config
	aggregations []*Aggregation
	counter      *prometheus.CounterVec
	cache        map[string]*cacheEntry
	mutex        *sync.Mutex
}

// A cached metric consists of the metric and a ttl in seconds
type cacheEntry struct {
	m   []prometheus.Metric
	ttl int64
}

// A server needs a driver (implementation) and a unique name
type server struct {
	name   string
	driver Driver
}

type option func(c *Collector)

// Collector configuration with default metric configurations
type Config struct {
	QueryTimeout      time.Duration
	DefaultCache      time.Duration
	DefaultMode       string
	DefaultDatabase   string
	DefaultCollection string
}

// Aggregation defines what aggregation pipeline is executed on what servers
type Aggregation struct {
	Servers    []string
	Cache      time.Duration
	Mode       string
	Database   string
	Collection string
	Pipeline   string
	Metrics    []*Metric
	pipeline   bson.A
}

// A metric defines how a certain value is exported from a MongoDB aggregation
type Metric struct {
	Name          string
	Type          string
	Help          string
	Value         string
	OverrideEmpty bool
	EmptyValue    int64
	ConstLabels   prometheus.Labels
	Labels        []string
	desc          *prometheus.Desc
}

var (
	//Only Gauge is a supported metric types
	ErrInvalidType = errors.New("unknown metric type provided. Only gauge is supported")
	
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
	//Metric generated successfully
	ResultSuccess = "SUCCESS"
	//Metric value could not been determined
	ResultError = "ERROR"
)

// Create a new collector
func New(opts ...option) *Collector {
	c := &Collector{
		logger: &dummyLogger{},
		config: &Config{
			QueryTimeout: 10 * time.Second,
		},
	}

	c.cache = make(map[string]*cacheEntry)
	c.mutex = &sync.Mutex{}

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
func (c *Collector) RegisterAggregation(aggregation *Aggregation) error {
	if len(aggregation.Servers) != 0 && len(aggregation.Servers) != len(c.GetServers(aggregation.Servers)) {
		return fmt.Errorf("aggregation bound to server which have not been found")
	}

	err := bson.UnmarshalExtJSON([]byte(aggregation.Pipeline), false, &aggregation.pipeline)
	if err != nil {
		return errors.Wrap(err, "failed to decode json aggregation pipeline")
	}

	if c.config.DefaultCache > 0 && aggregation.Cache != 0 {
		aggregation.Cache = c.config.DefaultCache
	}

	for _, metric := range aggregation.Metrics {
		c.logger.Debugf("register metric %s", metric.Name)
		metric.desc = c.describeMetric(metric)
	}

	c.aggregations = append(c.aggregations, aggregation)
	return nil
}

// Create prometheus descriptor
func (c *Collector) describeMetric(metric *Metric) *prometheus.Desc {
	return prometheus.NewDesc(
		metric.Name,
		metric.Help,
		append([]string{"server"}, metric.Labels...),
		metric.ConstLabels,
	)
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

	for _, aggregation := range c.aggregations {
		for _, metric := range aggregation.Metrics {
			ch <- metric.desc
		}
	}
}

// Collect all metrics from queries
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.logger.Debugf("start collecting metrics")
	var wg sync.WaitGroup

	for i, aggregation := range c.aggregations {
		for _, srv := range c.GetServers(aggregation.Servers) {
			metrics, err := c.getCached(aggregation, srv)

			if err == nil {
				c.logger.Debugf("use value from cache for %s", aggregation.Pipeline)

				for _, m := range metrics {
					ch <- m
				}
				continue
			}

			wg.Add(1)
			go func(i int, aggregation *Aggregation, srv *server, ch chan<- prometheus.Metric) {
				defer wg.Done()
				err := c.aggregate(aggregation, srv, ch)

				if c.counter == nil {
					return
				}

				var result string
				if err == nil {
					result = ResultSuccess
				} else {
					c.logger.Errorf("failed to generate metric", "err", err, "name", srv.name)

					result = ResultError
				}

				c.counter.With(prometheus.Labels{
					"server":      srv.name,
					"aggregation": fmt.Sprintf("aggregation_%d", i),
					"result":      result,
				}).Inc()
			}(i, aggregation, srv, ch)
		}
	}

	wg.Wait()

	if c.counter != nil {
		c.counter.Collect(ch)
	}
}

func (c *Collector) updateCache(aggregation *Aggregation, srv *server, m []prometheus.Metric) {
	var ttl int64

	if (aggregation.Mode == ModePush && aggregation.Cache == 0) || aggregation.Cache == -1 {
		c.logger.Debugf("cache metrics from aggregation %s until new push", aggregation.Pipeline)
		ttl = -1

	} else if aggregation.Cache > 0 {
		c.logger.Debugf("cache metris from aggregation %s for %d", aggregation.Pipeline, aggregation.Cache)
		ttl = time.Now().Unix() + int64(aggregation.Cache.Seconds())
	} else {
		c.logger.Debugf("skip caching metrics from aggregation %s", aggregation.Pipeline)
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.cache[aggregation.Pipeline+srv.name] = &cacheEntry{m, ttl}
}

func (c *Collector) getCached(aggregation *Aggregation, srv *server) ([]prometheus.Metric, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if e, exists := c.cache[aggregation.Pipeline+srv.name]; exists {
		if e.ttl == -1 || e.ttl >= time.Now().Unix() {
			return e.m, nil
		}

		// entry can be removed from cache since its expired
		delete(c.cache, aggregation.Pipeline+srv.name)
	}

	return nil, ErrNotCached
}

// Start MongoDB watchers for metrics where push is enabled.
// As soon as a new event is registered the cache gets invalidated and the aggregation
// will be re evaluated during the next scrape.
// This is a non blocking operation.
func (c *Collector) StartCacheInvalidator() error {
	for _, aggregation := range c.aggregations {
		if aggregation.Mode != ModePush {
			continue
		}

		for _, srv := range c.GetServers(aggregation.Servers) {
			go func(aggregation *Aggregation, srv *server) {
				err := c.pushUpdate(aggregation, srv)

				if err != nil {
					c.logger.Errorf("%s; failed to watch for updates, fallback to pull", err)
				}
			}(aggregation, srv)
		}
	}

	return nil
}

func (c *Collector) pushUpdate(aggregation *Aggregation, srv *server) error {
	ctx := context.Background()

	c.logger.Infof("start changestream on %s.%s, waiting for changes", aggregation.Database, aggregation.Collection)
	cursor, err := srv.driver.Watch(ctx, aggregation.Database, aggregation.Collection, bson.A{})

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
		c.mutex.Lock()
		delete(c.cache, aggregation.Pipeline+srv.name)
		c.mutex.Unlock()
	}

	return nil
}

func (c *Collector) aggregate(aggregation *Aggregation, srv *server, ch chan<- prometheus.Metric) error {
	c.logger.Debugf("run aggregation %s on server %s", aggregation.Pipeline, srv.name)

	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeout)
	defer cancel()

	cursor, err := srv.driver.Aggregate(ctx, aggregation.Database, aggregation.Collection, aggregation.pipeline)
	if err != nil {
		return err
	}

	var multierr *multierror.Error
	var i int
	var result = make(AggregationResult)
	var metrics []prometheus.Metric

	for cursor.Next(ctx) {
		i++

		err := cursor.Decode(&result)
		c.logger.Debugf("found record %s from aggregation %s", result, aggregation.Pipeline)

		if err != nil {
			multierr = multierror.Append(multierr, err)
			c.logger.Errorf("failed decode record %s", err)
			continue
		}

		for _, metric := range aggregation.Metrics {
			m, err := createMetric(srv, metric, result)
			if err != nil {
				return err
			}

			metrics = append(metrics, m)
			ch <- m
		}
	}

	if i == 0 {
		for _, metric := range aggregation.Metrics {
			if !metric.OverrideEmpty {
				c.logger.Debugf("skip metric %s with an empty result from aggregation %s", metric.Name, aggregation.Pipeline)
				continue
			}

			result[metric.Value] = int64(metric.EmptyValue)
			for _, label := range metric.Labels {
				result[label] = ""
			}

			m, err := createMetric(srv, metric, result)
			if err != nil {
				return err
			}

			ch <- m
		}
	}

	c.updateCache(aggregation, srv, metrics)
	return multierr.ErrorOrNil()
}

func createMetric(srv *server, metric *Metric, result AggregationResult) (prometheus.Metric, error) {
	var (
		value float64
		err   error
	)

	if metric.Value == "" && metric.OverrideEmpty {
		value = float64(metric.EmptyValue)
	} else {
		value, err = metric.getValue(result)
	}

	if err != nil {
		return nil, err
	}

	labels, err := metric.getLabels(result)
	if err != nil {
		return nil, err
	}

	labels = append([]string{srv.name}, labels...)
	return prometheus.NewConstMetric(metric.desc, prometheus.GaugeValue, value, labels...)
}

func (metric *Metric) getValue(result AggregationResult) (float64, error) {
	if val, ok := result[metric.Value]; ok {
		switch v := val.(type) {
		case float32:
			value := float64(v)
			return value, nil
		case float64:
			return v, nil
		case int32:
			value := float64(v)
			return value, nil
		case int64:
			value := float64(v)
			return value, nil
		default:
			return 0, fmt.Errorf("provided value taken from the aggregation result has to be a number, type %T given", val)
		}
	}

	return 0, fmt.Errorf("value for metric %s with key %s not found in result set", metric.Name, metric.Value)
}

func (metric *Metric) getLabels(result AggregationResult) ([]string, error) {
	var labels []string

	for _, label := range metric.Labels {
		if val, ok := result[label]; ok {
			switch v := val.(type) {
			case string:
				labels = append(labels, v)
			default:
				return labels, fmt.Errorf("provided label value taken from the aggregation result has to be a string, type %T given", val)
			}
		} else {
			return labels, fmt.Errorf("required label %s not found in result set", label)
		}
	}

	return labels, nil
}
