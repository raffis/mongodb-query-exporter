package collector

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/tj/assert"
)

func buildMockDriver(docs []interface{}) *mockMongoDBDriver {
	return &mockMongoDBDriver{
		AggregateCursor: &mockCursor{
			Data: docs,
		},
	}
}

type metricTest struct {
	name           string
	counter        bool
	metric         *Metric
	error          string
	expected       string
	expectedCached string
	docs           []interface{}
}

func TestInitializeMetrics(t *testing.T) {
	var tests = []metricTest{
		metricTest{
			name: "Metric with no type should fail in unsupported metric type",
			metric: &Metric{
				Name: "simple_unlabled_notype",
			},
			error: "failed to initialize metric simple_unlabled_notype with error unknown metric type provided. Only [gauge] are valid options",
		},
		metricTest{
			name: "Metric with invalid type should fail in unsupported metric type",
			metric: &Metric{
				Name: "simple_unlabled_invalidtype",
				Type: "notexists",
			},
			error: "failed to initialize metric simple_unlabled_invalidtype with error unknown metric type provided. Only [gauge] are valid options",
		},
		metricTest{
			name: "Invalid aggregation pipeline must end in error",
			metric: &Metric{
				Name:     "simple_gauge_no_pipeline",
				Type:     "gauge",
				Pipeline: "{",
			},
			error: "failed to decode json aggregation pipeline: invalid JSON input",
		},
		metricTest{
			name: "Constant labeled gauge and valid value results in a success",
			metric: &Metric{
				Name:        "simple",
				Type:        "gauge",
				Value:       "total",
				Help:        "foobar",
				ConstLabels: prometheus.Labels{"foo": "bar"},
				Pipeline:    "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
			expected: `
				# HELP simple foobar
				# TYPE simple gauge
				simple{foo="bar"} 1
			`,
		},
		metricTest{
			name: "Unlabeled gauge and valid value results in a success",
			metric: &Metric{
				Name:     "simple",
				Type:     "gauge",
				Value:    "total",
				Help:     "foobar",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(2),
			}},
			expected: `
				# HELP simple foobar
				# TYPE simple gauge
				simple 2
			`,
		},
		metricTest{
			name:    "Unlabeled gauge and valid value results in a success including successful counter",
			counter: true,
			metric: &Metric{
				Name:     "simple",
				Type:     "gauge",
				Value:    "total",
				Help:     "foobar",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(2),
			}},
			expected: `
			# HELP counter_total mongodb query stats
			# TYPE counter_total counter
			counter_total{metric="simple",result="SUCCESS",server="main"} 1
			# HELP simple foobar
			# TYPE simple gauge
			simple 2
			`,
		},
		metricTest{
			name: "Unlabeled gauge no value found in result",
			metric: &Metric{
				Name:     "simple_gauge_value_not_found",
				Type:     "gauge",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{}},
			//error: "1 error occurred:\n\t* value not found in result set\n\n",
			expected: ``,
		},
		metricTest{
			name: "Unlabeled gauge value not of type float",
			metric: &Metric{
				Name:     "simple_gauge_value_not_float",
				Type:     "gauge",
				Value:    "total",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:     []interface{}{AggregationResult{"total": "bar"}},
			expected: ``,
			//error: "1 error occurred:\n\t* provided value taken from the aggregation result has to be a number, type string given\n\n",
		},
		metricTest{
			name: "Labeled gauge labels not found in result",
			metric: &Metric{
				Name:     "simple_gauge_label_not_found",
				Type:     "gauge",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:     []interface{}{AggregationResult{"total": float64(1)}},
			expected: ``,
			//error: "1 error occurred:\n\t* required label foo not found in result set\n\n",
		},
		metricTest{
			name: "Labeled gauge with existing label but not as a string",
			metric: &Metric{
				Name:     "simple_gauge_non_string_label",
				Type:     "gauge",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			//error: "1 error occurred:\n\t* provided label value taken from the aggregation result has to be a string, type bool given\n\n",
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   true,
			}},
			expected: ``,
		},
		metricTest{
			name:    "Labeled gauge with existing label but not as a string with ERROR counter",
			counter: true,
			metric: &Metric{
				Name:     "simple_gauge_non_string_label",
				Type:     "gauge",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			//error: "1 error occurred:\n\t* provided label value taken from the aggregation result has to be a string, type bool given\n\n",
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   true,
			}},
			expected: `
			# HELP counter_total mongodb query stats
			# TYPE counter_total counter
			counter_total{metric="simple_gauge_non_string_label",result="ERROR",server="main"} 1
			`,
		},
		metricTest{
			name: "Labeled gauge with labels and valid value results in a success",
			metric: &Metric{
				Name:     "simple_gauge_label",
				Type:     "gauge",
				Help:     "foobar",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   "bar",
			}},
			expected: `
				# HELP simple_gauge_label foobar
				# TYPE simple_gauge_label gauge
				simple_gauge_label{foo="bar"} 1
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drv := buildMockDriver(test.docs)
			var c *Collector
			var counter *prometheus.CounterVec
			reg := prometheus.NewPedanticRegistry()

			if test.counter == true {
				counter = prometheus.NewCounterVec(
					prometheus.CounterOpts{
						Name: "counter_total",
						Help: "mongodb query stats",
					},
					[]string{"metric", "server", "result"},
				)

				assert.NoError(t, reg.Register(counter))
				c = New(WithCounter(counter))
			} else {
				c = New()
			}

			assert.NoError(t, c.RegisterServer("main", drv))

			if test.error != "" {
				assert.Error(t, c.RegisterMetric(test.metric))
				return
			}

			assert.NoError(t, reg.Register(c))
			assert.NoError(t, c.RegisterMetric(test.metric))

			/*ch := make(chan<- prometheus.Metric, 10)
			c.Collect(ch)*/
			assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(test.expected)))
		})
	}
}

func TestCachedMetric(t *testing.T) {
	var tests = []metricTest{
		metricTest{
			name: "Metric without cache (60s) provides a different value during the next scrape",
			metric: &Metric{
				Name:     "simple_gauge_no_cache",
				Type:     "gauge",
				Value:    "total",
				Cache:    0,
				Help:     "foobar",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
			expected: `
				# HELP simple_gauge_no_cache foobar
				# TYPE simple_gauge_no_cache gauge
				simple_gauge_no_cache 1
			`,
			expectedCached: `
				# HELP simple_gauge_no_cache foobar
				# TYPE simple_gauge_no_cache gauge
				simple_gauge_no_cache 2
			`,
		},
		metricTest{
			name: "Metric with cache (60s) provides the same value during the next scrape",
			metric: &Metric{
				Name:     "simple_gauge_cached",
				Type:     "gauge",
				Value:    "total",
				Help:     "Cached for 60s",
				Cache:    60,
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
			expected: `
				# HELP simple_gauge_cached Cached for 60s
				# TYPE simple_gauge_cached gauge
				simple_gauge_cached 1
			`,
			expectedCached: `
				# HELP simple_gauge_cached Cached for 60s
				# TYPE simple_gauge_cached gauge
				simple_gauge_cached 1
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drv := buildMockDriver(test.docs)
			c := New()

			assert.NoError(t, c.RegisterServer("main", drv))
			assert.NoError(t, c.RegisterMetric(test.metric))
			assert.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(test.expected)))

			// Set a new value before the next scrape
			test.docs[0] = AggregationResult{
				"total": float64(2),
			}
			assert.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(test.expectedCached)))
		})
	}
}
