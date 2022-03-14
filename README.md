# Prometheus MongoDB query exporter
[![.github/workflows/action.yml](https://github.com/raffis/mongodb-query-exporter/workflows/.github/workflows/action.yml/badge.svg)](https://github.com/raffis/mongodb-query-exporter/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/raffis/mongodb-query-exporter)](https://goreportcard.com/report/github.com/raffis/mongodb-query-exporter)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/raffis/mongodb-query-exporter?tab=subdirectories)](https://pkg.go.dev/github.com/raffis/mongodb-query-exporter?tab=subdirectories)
[![Coverage Status](https://coveralls.io/repos/github/raffis/mongodb-query-exporter/badge.svg?branch=master)](https://coveralls.io/github/raffis/mongodb-query-exporter?branch=master)
[![Docker Pulls](https://img.shields.io/docker/pulls/raffis/mongodb-query-exporter.svg?maxAge=604800)](https://hub.docker.com/r/raffis/mongodb-query-exporter)
[![Artifact HUB](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/prometheus-mongodb-query-exporter)](https://artifacthub.io/packages/search?repo=prometheus-mongodb-query-exporter)

MongoDB aggregation query exporter for [Prometheus](https://prometheus.io).

## Features

* Support for gauge metrics
* Pull and Push (Push is only supported for MongoDB >= 3.6)
* Supports multiple MongoDB servers
* Metric caching support

Note that this is not designed to be a replacement for the [MongoDB exporter](https://github.com/percona/mongodb_exporter) to instrument MongoDB internals. This application exports custom MongoDB metrics in the prometheus format based on the queries (aggregations) you want.

## Installation

Get Prometheus MongoDB aggregation query exporter, either as a [binary](https://github.com/raffis/mongodb-query-exporter/releases/latest) or packaged as a [Docker image](https://hub.docker.com/r/raffis/mongodb-query-exporter).

### Helm Chart
For kubernetes users there is an official helm chart for the MongoDB query exporter.
Please read the installation instructions [here](https://github.com/raffis/mongodb-query-exporter/blob/master/chart/prometheus-mongodb-query-exporter/README.md).

## Usage

```
$ mongodb_query_exporter
```

Use the `-help` flag to get help information.

If you use [MongoDB Authorization](https://docs.mongodb.org/manual/core/authorization/), best practices is to create a dedicated readonly user with access to all databases/collections required:

1. Create a user with '*read*' on your database, like the following (*replace username/password/db!*):

```js
db.getSiblingDB("admin").createUser({
    user: "mongodb_query_exporter",
    pwd: "secret",
    roles: [
        { role: "read", db: "mydb" }
    ]
})
```

2. Set environment variable `MONGODB_URI` before starting the exporter:

```bash
export MDBEXPORTER_MONGODB_URI=mongodb://mongodb_query_exporter:secret@localhost:27017
```

If you use [x.509 Certificates to Authenticate Clients](https://docs.mongodb.com/manual/tutorial/configure-x509-client-authentication/), pass in username and `authMechanism` via [connection options](https://docs.mongodb.com/manual/reference/connection-string/#connections-connection-options) to the MongoDB uri. Eg:

```
mongodb://CN=myName,OU=myOrgUnit,O=myOrg,L=myLocality,ST=myState,C=myCountry@localhost:27017/?authMechanism=MONGODB-X509
```

## Access metrics
The metrics are by default exposed at `/metrics`.

```
curl localhost:9412/metrics
```

## Configuration

The exporter is looking for a configuration in `~/config.yaml` and `/etc/mongodb-query-exporter/config.yaml` or if set the path from the env `MDBEXPORTER_CONFIG`.

You may also use env variables to configure the exporter:

| Env variable             | Description                              |
|--------------------------|------------------------------------------|
| MDBEXPORTER_CONFIG       | Custom path for the configuration        |
| MDBEXPORTER_MONGODB_URI  | The MongoDB connection URI               |
| MDBEXPORTER_MONGODB_QUERY_TIMEOUT | Timeout until a MongoDB operations gets aborted |
| MDBEXPORTER_LOG_LEVEL    | Log level                                |
| MDBEXPORTER_LOG_ENCODING | Log format                               |
| MDBEXPORTER_BIND         | Bind address for the HTTP server         |
| MDBEXPORTER_METRICSPATH  | Change the metrics path (/metrics)       |

Note if you have multiple MongoDB servers you can inject an env variable for each instead using `MDBEXPORTER_MONGODB_URI`:

1. `MDBEXPORTER_SERVER_0_MONGODB_URI=mongodb://srv1:27017`
2. `MDBEXPORTER_SERVER_1_MONGODB_URI=mongodb://srv2:27017`
3. ...

### Example

The config format v2.0 is not supported in any version before `v1.0.0-beta5`. Please use v1.0 or upgrade to the latest version otherwise.
Starting with v1.0.0-beta5 the v2.0 format is the preferred version.

Example:
```yaml
version: 2.0
bind: 0.0.0.0:9412
log:
  encoding: json
  level: info
  development: false
  disableCaller: false
global:
  queryTimeout: 10
  maxConnection: 3
  defaultCache: 0
servers:
- name: main
  uri: mongodb://localhost:27017
metrics:
- name: myapp_example_simplevalue_total
  type: gauge #Can also be empty, the default is gauge
  servers: [main] #Can also be empty, if empty the metric will be used for every server defined
  help: 'Simple gauge metric'
  value: total
  labels: []
  mode: pull
  cache: 0
  database: mydb
  collection: objects
  pipeline: |
    [
      {"$count":"total"}
    ]
- name: myapp_example_processes_total
  type: gauge
  help: 'The total number of processes in a job queue'
  value: total
  mode: push
  labels: [type,status]
  constLabels:
    app: foo
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
              "default": "unknown"
          }}
      }}
    ]
```

See more examples in the `/example` folder.

## Cache & Push
Prometheus is designed to scrape metrics. During each scrape the mongodb-query-exporter will evaluate all configured metrics.
If you have expensive queries there is an option to cache the aggregation result by setting a cache ttl in secconds.
However it is more effective to **avoid cache** and design good aggregation pipelines. In some cases a different scrape interval might also be a solution.
For individual metrics and/or MongoDB servers older than 3.6 it might still be a good option though.

A better approach is using push instead a static cache, see bellow.

Example:
```yaml
metrics:
- name: myapp_example_simplevalue_total
  servers: [main]
  help: 'Simple gauge metric which is cached for 5min'
  value: total
  mode: pull
  cache: 300
  database: mydb
  collection: objects
  pipeline: |
    [
      {"$count":"total"}
    ]
```

To reduce load on the MongoDB server (and also scrape time) there is a push mode. Push automatically caches the metric at scrape time preferred (If no cache ttl is set). However the cache for a metric with mode push
will be invalidated automatically if anything changes within the configured MongoDB collection. Meaning the aggregation will only be executed if there have been changes during scrape intervals.

>**Note**: This requires at least MongoDB 3.6.

Example:
```yaml
metrics:
# With the mode push the pipeline is only executed if a change occured on the collection called objects
- name: myapp_example_simplevalue_total
  servers: [main]
  help: 'Simple gauge metric'
  value: total
  mode: push
  database: mydb
  collection: objects
  pipeline: |
    [
      {"$count":"total"}
    ]
```

## Debug
The mongodb-query-exporters also publishes a counter metric called `mongodb_query_exporter_query_total` which counts query results for each configured metric.
Furthermore you might increase the log level to get more insight.

## Used by
* The balloon helm chart implements the mongodb-query-exporter to expose general stats from the MongoDB like the number of total nodes or files stored internally or externally.
See the [config-map here](https://github.com/gyselroth/balloon-helm/blob/master/unstable/balloon/charts/balloon-mongodb-metrics/templates/config-map.yaml).


Please submit a PR if your project should be listed here!
