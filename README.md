# Prometheus MongoDB query exporter
[![Build Status](https://travis-ci.org/raffis/mongodb-query-exporter.svg?branch=master)](https://travis-ci.org/raffis/mongodb-query-exporter)
[![Go Report Card](https://goreportcard.com/badge/github.com/raffis/mongodb-query-exporter)](https://goreportcard.com/report/github.com/raffis/mongodb-query-exporter)
[![GoDoc](https://godoc.org/github.com/raffis/mongodb_query_exporter?status.svg)](https://godoc.org/github.com/raffis/mongodb-query-exporter)
[![Docker Pulls](https://img.shields.io/docker/pulls/githubraffis/mongodb-query-exporter.svg?maxAge=604800)](https://hub.docker.com/r/githubraffis/mongodb-query-exporter)

MongoDB aggregation query exporter for [Prometheus](https://prometheus.io).


## Usage

Get Prometheus MongoDB aggregation query exporter, either as a [binary](https://github.com/raffis/mongodb-query-exporter/releases/latest) or packaged as a [Docker image](https://hub.docker.com/r/githubraffis/mongodb-query-exporter).

```
$ mongodb_query_exporter
```

Use the `-help` flag to get help information.

```
Export different aggregations from MongoDB as prometheus comptatible metrics.

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



**`./config.yml`**

```yaml
bind: 0.0.0.0:9412
logLevel: info
mongodb:
  uri: mongodb://localhost:27017
  connection_timeout: 10
  max_connection: 3
  defaultCacheTime: 5
metrics:
- name: myapp_simplevalue_total
  type: gauge
  help: 'Simple gauge metric'
  value: total
  cacheTime: 10
  database: mydb
  collection: objects
  pipeline: |
    [
      {"$count":"total"}
    ]  
- name: myapp_processes_total
  type: gauge
  help: 'The total number of processes in the queuy labeled by their status'
  value: total
  cacheTime: 5
  labels: [type,status]
  database: mydb
  collection: queue
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
              "default": { "then": "unknown" }
          }}
      }}
    ]
```
