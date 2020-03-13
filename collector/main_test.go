package collector

import (
	"io/ioutil"
	"testing"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func buildMock(docs []interface{}) *mockMongoDBDriver {
	return &mockMongoDBDriver{
		AggregateCursor: &mockCursor{
			Data: docs,
		},
	}
}

type metricTest struct {
	name   string
	metric *Metric
	error  string
	docs   []interface{}
}

func TestInitializeMetrics(t *testing.T) {
	var tests = []metricTest{
		metricTest{
			name: "Metric with no type should fail in unsupported metric type",
			metric: &Metric{
				Name: "simple_unlabled_notype",
			},
			error: "failed to initialize metric simple_unlabled_notype with error unknown metric type provided. Only [gauge,counter] are valid options",
		},
		metricTest{
			name: "Metric with no type should fail in unsupported metric type",
			metric: &Metric{
				Name: "simple_unlabled_invalidtype",
				Type: "notexists",
			},
			error: "failed to initialize metric simple_unlabled_invalidtype with error unknown metric type provided. Only [gauge,counter] are valid options",
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
			name: "Invalid aggregation pipeline must end in error",
			metric: &Metric{
				Name:     "simple_counter_no_pipeline",
				Type:     "counter",
				Pipeline: "{",
			},
			error: "failed to decode json aggregation pipeline: invalid JSON input",
		},
		metricTest{
			name: "Unlabeled gauge no value found in result",
			metric: &Metric{
				Name:     "simple_gauge_value_not_found",
				Type:     "gauge",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:  []interface{}{AggregationResult{}},
			error: "1 error occurred:\n\t* value not found in result set\n\n",
		},
		metricTest{
			name: "Unlabeled counter no value found in result",
			metric: &Metric{
				Name:     "simple_counter_value_not_found",
				Type:     "counter",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:  []interface{}{AggregationResult{}},
			error: "1 error occurred:\n\t* value not found in result set\n\n",
		},
		metricTest{
			name: "Unlabeled gauge value not of type float",
			metric: &Metric{
				Name:     "simple_gauge_value_not_float",
				Type:     "gauge",
				Value:    "total",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:  []interface{}{AggregationResult{"total": "bar"}},
			error: "1 error occurred:\n\t* provided value taken from the aggregation result has to be a number, type string given\n\n",
		},
		metricTest{
			name: "Unlabeled counter value not of type float",
			metric: &Metric{
				Name:     "simple_counter_value_not_float",
				Type:     "counter",
				Value:    "total",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:  []interface{}{AggregationResult{"total": "bar"}},
			error: "1 error occurred:\n\t* provided value taken from the aggregation result has to be a number, type string given\n\n",
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
			docs:  []interface{}{AggregationResult{"total": float64(1)}},
			error: "1 error occurred:\n\t* required label foo not found in result set\n\n",
		},
		metricTest{
			name: "Labeled counter labels not found in result",
			metric: &Metric{
				Name:     "simple_counter_label_not_found",
				Type:     "counter",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs:  []interface{}{AggregationResult{"total": float64(1)}},
			error: "1 error occurred:\n\t* required label foo not found in result set\n\n",
		},
		metricTest{
			name: "Unlabeled gauge and valid value results in a success",
			metric: &Metric{
				Name:     "simple_gauge",
				Type:     "gauge",
				Value:    "total",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
			}},
		},
		metricTest{
			name: "counter and valid value results in a success",
			metric: &Metric{
				Name:     "simple_counter",
				Type:     "counter",
				Value:    "total",
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(10),
			}},
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
			error: "1 error occurred:\n\t* provided label value taken from the aggregation result has to be a string, type bool given\n\n",
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   true,
			}},
		},
		metricTest{
			name: "Labeled gauge with labels and valid value results in a success",
			metric: &Metric{
				Name:     "simple_gauge_label",
				Type:     "gauge",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(1),
				"foo":   "bar",
			}},
		},
		metricTest{
			name: "Labeled counter with labels and valid value results in a success",
			metric: &Metric{
				Name:     "simple_counter_label",
				Type:     "counter",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			docs: []interface{}{AggregationResult{
				"total": float64(10),
				"foo":   "bar",
			}},
		},
		metricTest{
			name: "Labeled counter with labels fails while increase counter by the same value",
			metric: &Metric{
				Name:     "simple_counter_label_same_increase",
				Type:     "counter",
				Value:    "total",
				Labels:   []string{"foo"},
				Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			error: "1 error occurred:\n\t* failed to increase counter for simple_counter_label_same_increase, counter can not be decreased\n\n",
			docs: []interface{}{
				AggregationResult{
					"total": float64(10),
					"foo":   "bar",
				},
				AggregationResult{
					"total": float64(9),
					"foo":   "bar",
				}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector := &Collector{buildMock(test.docs), &Config{
				Metrics: []*Metric{test.metric},
			}}

			err := collector.initializeMetric(test.metric)

			if test.error == "" && err == nil {
				return
			}

			if err == nil {
				t.Error("expected error, got none")
				return
			}

			actual := err.Error()
			if actual != test.error {
				t.Errorf("expected '%s', but got '%s'", test.error, err.Error())
			}
		})
	}
}

func TestInitializeMetricStack(t *testing.T) {
	t.Run("Initialize metric stack one of two but only one is valid", func(t *testing.T) {
		var d []interface{}

		collector := &Collector{buildMock(d), &Config{
			Metrics: []*Metric{
				&Metric{
					Name:     "foobar",
					Type:     "counter",
					Value:    "total",
					Labels:   []string{"foo"},
					Pipeline: "[{\"$match\":{\"foo\":\"bar\"}}]",
				},
				&Metric{
					Name: "not_exising",
					Type: "notexising",
				},
			},
		}}

		collector.initializeMetrics()

		if collector.Config.Metrics[0].metric == nil {
			t.Errorf("expected initialized metric, but got nil")
		}

		if collector.Config.Metrics[1].metric != nil {
			t.Errorf("expected uninitialized metric, but got an initialized metric")
		}
	})
}

func TestEventstreamMetrics(t *testing.T) {
	var tests = []metricTest{
		metricTest{
			name: "Successful eventstream with uninitialized metric (no aggregation result)",
			metric: &Metric{
				Name:       "simple_counter_realtime_update",
				Type:       "counter",
				Value:      "total",
				Realtime:   true,
				Database:   "foo",
				Collection: "bar",
				Labels:     []string{"foo"},
				Pipeline:   "[{\"$match\":{\"foo\":\"bar\"}}]",
			},
			error: "1 error occurred:\n\t* failed to update metric simple_counter_realtime_update, failed with error metric simple_counter_realtime_update aggregation returned an emtpy result set\n\n",
			docs: []interface{}{
				ChangeStreamEvent{
					NS: &ChangeStreamEventNamespace{"foo", "bar"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector := &Collector{buildMock(test.docs), &Config{
				Metrics: []*Metric{test.metric},
			}}

			err := collector.realtimeUpdate(test.metric)

			if test.error == "" && err == nil {
				return
			}

			if err == nil {
				t.Error("expected error, got none")
				return
			}

			actual := err.Error()
			if actual != test.error {
				t.Errorf("expected '%s', but got '%s'", test.error, err.Error())
			}
		})
	}
}
