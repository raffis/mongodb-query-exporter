# Prometheus MongoDB query exporter
[![Build Status](https://travis-ci.org/raffis/mongodb-query-exporter.svg?branch=master)](https://travis-ci.org/raffis/mongodb-query-exporter)
[![Go Report Card](https://goreportcard.com/badge/github.com/raffis/mongodb-query-exporter)](https://goreportcard.com/report/github.com/raffis/mongodb-query-exporter)
[![GoDoc](https://godoc.org/github.com/raffis/mongodb_query_exporter?status.svg)](https://godoc.org/github.com/raffis/mongodb-query-exporter)
[![Coverage Status](https://coveralls.io/repos/github/raffis/mongodb-query-exporter/badge.svg?branch=master)](https://coveralls.io/github/raffis/mongodb-query-exporter?branch=master)
[![Docker Pulls](https://img.shields.io/docker/pulls/raffis/mongodb-query-exporter.svg?maxAge=604800)](https://hub.docker.com/r/raffis/mongodb-query-exporter)

MongoDB aggregation query exporter for [Prometheus](https://prometheus.io).

## Features

* Support for gauge and counter metrics
* Multiple metrics for different db/collections
* Pull interval
* Support for (soft) realtime metric updates (>= MongoDB 3.6)

## Beta notice

This software is currently beta and the API/configuration may break without notice until a stable version is released.

## Usage

Get Prometheus MongoDB aggregation query exporter, either as a [binary](https://github.com/raffis/mongodb-query-exporter/releases/latest) or packaged as a [Docker image](https://hub.docker.com/r/raffis/mongodb-query-exporter).

Note that this is not a replacement form the MongoDB exporter. This app allows it to expose custom MongoDB metrics based on configured aggregations and not a general point of view like the MongoDB exporter.


```
$ mongodb_query_exporter
```

Use the `-help` flag to get help information.

```
Export different aggregations from MongoDB as prometheus compatible metrics.

Usage:
  mongodb_query_exporter [flags]

Flags:
  -b, --bind string        config file (default is :9412) (default ":9412")
  -c, --config string      config file (default is $HOME/.mongodb_query_exporter/config.yaml)
  -h, --help               help for mongodb_query_exporter
  -l, --log-level string   Define a log level (default is info) (default "info")
  -t, --timeout int        MongoDB connection timeout (default is 10 secconds (default 10)
  -u, --uri string         MongoDB URI (default is mongodb://localhost:27017) (default "mongodb://localhost:27017")
```

## Configuration

Usually you want to deploy the MongoDB query exporter alongside the DB server it collects metrics from.
Alternatively you might also deploy it as a normal service and fetch aggregations from a whole replicaset.
If the provided MongoDB URI is not reachable by the exporter /metrics will report a HTTP code 500 Internal Server Error,
causing Prometheus to record `up=0` for that scrape.


Example:
**`./config.yml`**

```yaml
bind: 0.0.0.0:9412
logLevel: info
mongodb:
  uri: mongodb://localhost:27017
  connectionTimeout: 10
  maxConnection: 3
metrics:
- name: myapp_example_simplevalue_total
  type: gauge
  help: 'Simple gauge metric'
  value: total
  labels: []
  cacheTime: 10
  constLabels: []
  database: mydb
  collection: objects  
  realtime: false
  pipeline: |
    [
      {"$count":"total"}
    ]  
- name: myapp_example_processes_total
  type: gauge
  help: 'The total number of processes in a job queue'
  value: total
  cacheTime: 5
  labels: [type,status]
  constLabels: []
  database: mydb
  collection: queue
  realtime: true
  pipeline: |
    [
      {"$group": {
        "_id":{"status":"$status","name":"$class"},
        "total":{"$sum":1}
      }},
      {"$project":{
        "_id":0,
        "type":"$_id.name",
        "total":"$total",
        "status": {
          "$switch": {
              "branches": [
                 { "case": { "$eq": ["$_id.status", 0] }, "then": "waiting" },
                 { "case": { "$eq": ["$_id.status", 1] }, "then": "postponed" },
                 { "case": { "$eq": ["$_id.status", 2] }, "then": "processing" },
                 { "case": { "$eq": ["$_id.status", 3] }, "then": "done" },
                 { "case": { "$eq": ["$_id.status", 4] }, "then": "failed" },
                 { "case": { "$eq": ["$_id.status", 5] }, "then": "canceled" },
                 { "case": { "$eq": ["$_id.status", 6] }, "then": "timeout" }
              ],
              "default": "unknown"
          }}
      }}
    ]
```

### Real world example
The balloon helm chart implements the mongodb-query-exporter to expose general stats from the MongoDB like the number of total nodes or files stored internally or externally.
See the [config-map here](https://github.com/gyselroth/balloon-helm/blob/master/unstable/balloon/charts/balloon-mongodb-metrics/templates/config-map.yaml).
