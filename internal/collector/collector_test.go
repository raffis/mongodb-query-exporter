package collector

import (
	"strings"
	"testing"
	"time"

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

type aggregationTest struct {
	name           string
	counter        bool
	aggregation    *Aggregation
	error          string
	expected       string
	expectedCached string
	docs           []interface{}
}

func TestInitializeMetrics(t *testing.T) {
	var tests = []aggregationTest{
		{
			name: "Metric with no type should fail in unsupported metric type",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name: "simple_unlabled_notype",
					},
				},
			},
			error: "failed to initialize metric simple_unlabled_notype with error unknown metric type provided. Only [gauge] are valid options",
		},
		{
			name: "Metric with invalid type should fail in unsupported metric type",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name: "simple_unlabled_invalidtype",
						Type: "notexists",
					},
				},
			},
			error: "failed to initialize metric simple_unlabled_invalidtype with error unknown metric type provided. Only [gauge] are valid options",
		},
		{
			name: "Invalid aggregation pipeline must end in error",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name: "simple_gauge_no_pipeline",
						Type: "gauge",
					},
				},
				Pipeline: "{",
			},
			error: "failed to decode json aggregation pipeline: invalid JSON input",
		},
		{
			name: "Constant labeled gauge and valid value results in a success",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:        "simple",
						Type:        "gauge",
						Value:       "total",
						Help:        "foobar",
						ConstLabels: prometheus.Labels{"foo": "bar"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
			expected: `
				# HELP simple foobar
				# TYPE simple gauge
				simple{foo="bar",server="main"} 1
			`,
		},
		{
			name: "Unlabeled gauge and valid value results in a success",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:  "simple",
						Type:  "gauge",
						Value: "total",
						Help:  "foobar",
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(2),
			}},
			expected: `
				# HELP simple foobar
				# TYPE simple gauge
				simple{server="main"} 2
			`,
		},
		{
			name:    "Unlabeled gauge and valid value results in a success including successful counter",
			counter: true,
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:  "simple",
						Type:  "gauge",
						Value: "total",
						Help:  "foobar",
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(2),
			}},
			expected: `
			# HELP counter_total mongodb query stats
			# TYPE counter_total counter
			counter_total{aggregation="aggregation_0",result="SUCCESS",server="main"} 1
			# HELP simple foobar
			# TYPE simple gauge
			simple{server="main"} 2
			`,
		},
		{
			name: "Unlabeled gauge no value found in result",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:  "simple_gauge_value_not_found",
						Type:  "gauge",
						Value: "total",
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{}},
			//error: "value for metric simple_gauge_value_not_found with key total not found in result set",
			expected: ``,
		},
		{
			name: "Unlabeled gauge no value found in result but OverrideEmpty is set with EmptyValue 0",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:          "simple_gauge_value_not_found_overridden",
						Type:          "gauge",
						Help:          "overridden",
						OverrideEmpty: true,
						EmptyValue:    12,
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			expected: `
				# HELP simple_gauge_value_not_found_overridden overridden
				# TYPE simple_gauge_value_not_found_overridden gauge
				simple_gauge_value_not_found_overridden{server="main"} 12
			`,
		},
		{
			name: "Unlabeled gauge value not of type float",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:  "simple_gauge_value_not_float",
						Type:  "gauge",
						Value: "total",
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:     []interface{}{AggregationResult{"total": "bar"}},
			expected: ``,
			//error: "1 error occurred:\n\t* provided value taken from the aggregation result has to be a number, type string given\n\n",
		},
		{
			name: "Labeled gauge labels not found in result",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:   "simple_gauge_label_not_found",
						Type:   "gauge",
						Value:  "total",
						Labels: []string{"foo"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:     []interface{}{AggregationResult{"total": float64(1)}},
			expected: ``,
			//error: "1 error occurred:\n\t* required label foo not found in result set\n\n",
		},
		{
			name: "Labeled gauge with existing label but not as a string",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:   "simple_gauge_non_string_label",
						Type:   "gauge",
						Value:  "total",
						Labels: []string{"foo"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			//error: "1 error occurred:\n\t* provided label value taken from the aggregation result has to be a string, type bool given\n\n",
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   true,
			}},
			expected: ``,
		},
		{
			name:    "Labeled gauge with existing label but not as a string with ERROR counter",
			counter: true,
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:   "simple_gauge_non_string_label",
						Type:   "gauge",
						Value:  "total",
						Labels: []string{"foo"},
					},
				},
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
			counter_total{aggregation="aggregation_0",result="ERROR",server="main"} 1
			`,
		},
		{
			name: "Labeled gauge with labels and valid value results in a success",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:   "simple_gauge_label",
						Type:   "gauge",
						Help:   "foobar",
						Value:  "total",
						Labels: []string{"foo"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   "bar",
			}},
			expected: `
				# HELP simple_gauge_label foobar
				# TYPE simple_gauge_label gauge
				simple_gauge_label{foo="bar",server="main"} 1
			`,
		},
		{
			name: "Empty result and overrideEmpty is not set results in no metric",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:   "no_result",
						Type:   "gauge",
						Help:   "foobar",
						Value:  "total",
						Labels: []string{"foo"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:     []interface{}{},
			expected: ``,
		},
		{
			name: "Metric without a value but overrideEmpty is still created",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:          "simple_info_metric",
						Type:          "gauge",
						Help:          "foobar",
						OverrideEmpty: true,
						EmptyValue:    1,
						Labels:        []string{"foo"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"foo": "bar",
			}},
			expected: `
				# HELP simple_info_metric foobar
				# TYPE simple_info_metric gauge
				simple_info_metric{foo="bar",server="main"} 1
			`,
		},
		{
			name: "Export multiple metrics from the same aggregation",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:   "simple_gauge_label",
						Type:   "gauge",
						Help:   "foobar",
						Value:  "total",
						Labels: []string{"foo"},
					},
					{
						Name:        "simple_gauge_label_with_constant",
						Type:        "gauge",
						Help:        "bar",
						Value:       "total",
						Labels:      []string{"foo"},
						ConstLabels: prometheus.Labels{"foobar": "foo"},
					},
				},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   "bar",
			}},
			expected: `
				# HELP simple_gauge_label foobar
				# TYPE simple_gauge_label gauge
				simple_gauge_label{foo="bar",server="main"} 1

				# HELP simple_gauge_label_with_constant bar
				# TYPE simple_gauge_label_with_constant gauge
				simple_gauge_label_with_constant{foo="bar",foobar="foo",server="main"} 1
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drv := buildMockDriver(test.docs)
			var c *Collector
			var counter *prometheus.CounterVec
			reg := prometheus.NewRegistry()

			if test.counter == true {
				counter = prometheus.NewCounterVec(
					prometheus.CounterOpts{
						Name: "counter_total",
						Help: "mongodb query stats",
					},
					[]string{"aggregation", "server", "result"},
				)

				c = New(WithCounter(counter))
			} else {
				c = New()
			}

			assert.NoError(t, c.RegisterServer("main", drv))

			if test.error != "" {
				assert.Error(t, c.RegisterAggregation(test.aggregation))
				return
			}

			assert.NoError(t, reg.Register(c))
			assert.NoError(t, c.RegisterAggregation(test.aggregation))
			assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(test.expected)))
		})
	}
}

func TestCachedMetric(t *testing.T) {
	var tests = []aggregationTest{
		{
			name: "Metric without cache (60s) provides a different value during the next scrape",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:  "simple_gauge_no_cache",
						Type:  "gauge",
						Value: "total",
						Help:  "foobar",
					},
				},
				Cache:    0,
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
			expected: `
				# HELP simple_gauge_no_cache foobar
				# TYPE simple_gauge_no_cache gauge
				simple_gauge_no_cache{server="main"} 1
			`,
			expectedCached: `
				# HELP simple_gauge_no_cache foobar
				# TYPE simple_gauge_no_cache gauge
				simple_gauge_no_cache{server="main"} 2
			`,
		},
		{
			name: "Metric with cache (60s) provides the same value during the next scrape",
			aggregation: &Aggregation{
				Metrics: []*Metric{
					{
						Name:  "simple_gauge_cached",
						Type:  "gauge",
						Value: "total",
						Help:  "Cached for 60s",
					},
				},
				Cache:    60 * time.Second,
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
			expected: `
				# HELP simple_gauge_cached Cached for 60s
				# TYPE simple_gauge_cached gauge
				simple_gauge_cached{server="main"} 1
			`,
			expectedCached: `
				# HELP simple_gauge_cached Cached for 60s
				# TYPE simple_gauge_cached gauge
				simple_gauge_cached{server="main"} 1
			`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drv := buildMockDriver(test.docs)
			c := New()

			assert.NoError(t, c.RegisterServer("main", drv))
			assert.NoError(t, c.RegisterAggregation(test.aggregation))
			assert.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(test.expected)))

			// Set a new value before the next scrape
			test.docs[0] = AggregationResult{
				"total": float64(2),
			}
			assert.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(test.expectedCached)))
		})
	}
}
