# Prometheus MongoDB query exporter
[![release](https://github.com/raffis/mongodb-query-exporter/actions/workflows/release.yaml/badge.svg)](https://github.com/raffis/mongodb-query-exporter/actions/workflows/release.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/raffis/mongodb-query-exporter/v5)](https://goreportcard.com/report/github.com/raffis/mongodb-query-exporter/v5)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/raffis/mongodb-query-exporter/badge)](https://api.securityscorecards.dev/projects/github.com/raffis/mongodb-query-exporter)
[![Coverage Status](https://coveralls.io/repos/github/raffis/mongodb-query-exporter/badge.svg?branch=master)](https://coveralls.io/github/raffis/mongodb-query-exporter?branch=master)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/mongodb-query-exporter)](https://artifacthub.io/packages/search?repo=mongodb-query-exporter)

MongoDB aggregation query exporter for [Prometheus](https://prometheus.io).

## Features

* Support for gauge metrics
* Pull and Push (Push is only supported for MongoDB >= 3.6)
* Supports multiple MongoDB servers
* Metric caching support

Note that this is not designed to be a replacement for the [MongoDB exporter](https://github.com/percona/mongodb_exporter) to instrument MongoDB internals. This application exports custom MongoDB metrics in the prometheus format based on the queries (aggregations) you want.

## Installation

Get Prometheus MongoDB aggregation query exporter, either as a binaray from the latest release or packaged as a [Docker image](https://github.com/raffis/mongodb-query-exporter/pkgs/container/mongodb-query-exporter).

### Helm Chart
For kubernetes users there is an official helm chart for the MongoDB query exporter.
Please read the installation instructions [here](https://github.com/raffis/mongodb-query-exporter/blob/master/chart/mongodb-query-exporter/README.md).

### Docker
You can run the exporter using docker (This will start it using the example config provided in the example folder):
```sh
docker run -e MDBEXPORTER_CONFIG=/config/configv3.yaml -v $(pwd)/example:/config ghcr.io/raffis/mongodb-query-exporter:latest
```

## Usage

```
$ mongodb-query-exporter
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

Note: The URI is substituted using env variables `${MY_ENV}`, given that you may also pass credentials from other env variables. See the example bellow.

If you use [x.509 Certificates to Authenticate Clients](https://docs.mongodb.com/manual/tutorial/configure-x509-client-authentication/), pass in username and `authMechanism` via [connection options](https://docs.mongodb.com/manual/reference/connection-string/#connections-connection-options) to the MongoDB uri. Eg:

```
mongodb://CN=myName,OU=myOrgUnit,O=myOrg,L=myLocality,ST=myState,C=myCountry@localhost:27017/?authMechanism=MONGODB-X509
```

## Credentials from env variables
You can pass in credentials from env variables.

Given the following URI the exporter will look for the ENV variables called `MY_USERNAME` and `MY_PASSWORD` and automatically use them at the referenced position within the URI.
```bash
export MY_USERNAME=mongodb_query_exporter
export MY_PASSWORD=secret
export MDBEXPORTER_MONGODB_URI=mongodb://${MY_USERNAME}:${MY_PASSWORD}@localhost:27017
```

## Access metrics
The metrics are by default exposed at `/metrics`.

```
curl localhost:9412/metrics
```

## Exporter configuration

The exporter is looking for a configuration in `~/.mongodb_query_exporter/config.yaml` and `/etc/mongodb_query_exporter/config.yaml` or if set the path from the env `MDBEXPORTER_CONFIG`.

You may also use env variables to configure the exporter:

| Env variable             | Description                              | Default |
|--------------------------|------------------------------------------|---------|
| MDBEXPORTER_CONFIG       | Custom path for the configuration        | `~/.mongodb_query_exporter/config.yaml` or `/etc/mongodb_query_exporter/config.yaml` |
| MDBEXPORTER_MONGODB_URI  | The MongoDB connection URI               | `mongodb://localhost:27017`
| MDBEXPORTER_MONGODB_QUERY_TIMEOUT | Timeout until a MongoDB operations gets aborted | `10` |
| MDBEXPORTER_LOG_LEVEL    | Log level                                | `warning` |
| MDBEXPORTER_LOG_ENCODING | Log format                               | `json` |
| MDBEXPORTER_BIND         | Bind address for the HTTP server         | `:9412` |
| MDBEXPORTER_METRICSPATH  | Metrics endpoint                         | `/metrics` |

Note if you have multiple MongoDB servers you can inject an env variable for each instead using `MDBEXPORTER_MONGODB_URI`:

1. `MDBEXPORTER_SERVER_0_MONGODB_URI=mongodb://srv1:27017`
2. `MDBEXPORTER_SERVER_1_MONGODB_URI=mongodb://srv2:27017`
3. ...

## Configure metrics

Since the v1.0.0 release you should use the config version v3.0 to profit from the latest features.
See the configuration version matrix bellow.

Example:
```yaml
version: 3.0
bind: 0.0.0.0:9412
log:
  encoding: json
  level: info
  development: false
  disableCaller: false
global:
  queryTimeout: 3s
  maxConnection: 3
  defaultCache: 0
servers:
- name: main
  uri: mongodb://localhost:27017
aggregations:
- database: mydb
  collection: objects
  servers: [main] #Can also be empty, if empty the metric will be used for every server defined
  metrics:
  - name: myapp_example_simplevalue_total
    type: gauge #Can also be empty, the default is gauge
    help: 'Simple gauge metric'
    value: total
    overrideEmpty: true # if an empty result set is returned..
    emptyValue: 0       # create a metric with value 0
    labels: []
    constLabels:
      region: eu-central-1
  cache: 0
  mode: pull
  pipeline: |
    [
      {"$count":"total"}
    ]
- database: mydb
  collection: queue
  metrics:
  - name: myapp_example_processes_total
    type: gauge
    help: 'The total number of processes in a job queue'
    value: total
    labels: [type,status]
    constLabels: {}
  mode: pull
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
- database: mydb
  collection: events
  metrics:
  - name: myapp_events_total
    type: gauge
    help: 'The total number of events (created 1h ago or newer)'
    value: count
    labels: [type]
    constLabels: {}
  mode: pull
  # Note $$NOW is only supported in MongoDB >= 4.2
  pipeline: |
    [
      { "$sort": { "created": -1 }},
      {"$limit": 100000},
      {"$match":{
        "$expr": {
          "$gte": [
            "$created",
            {
              "$subtract": ["$$NOW", 3600000]
            }
          ]
        }
      }},
      {"$group": {
        "_id":{"type":"$type"},
        "count":{"$sum":1}
      }},
      {"$project":{
        "_id":0,
        "type":"$_id.type",
        "count":"$count"
      }}
    ]
```

See more examples in the `/example` folder.

### Info metrics

By defining no actual value field but set `overrideEmpty` to `true` a metric can sill be exported
with labels from the aggregation pipeline but the value is set to a static value taken from `emptyValue`.
This is useful for exporting info metrics which can later be used for join queries.

```yaml
servers:
- name: main
  uri: mongodb://localhost:27017
aggregations:
- database: mydb
  collection: objects
  metrics:
  - name: myapp_info
    help: 'Info metric'
    overrideEmpty: true
    emptyValue: 1
    labels:
    - mylabel1
    - mylabel2
    constLabels:
      region: eu-central-1
  cache: 0
  mode: pull
  pipeline: `...`
```


## Supported config versions

| Config version           | Supported since   |
|--------------------------|-------------------|
| `v3.0`                   | v1.0.0            |
| `v2.0`                   | v1.0.0-beta5      |
| `v1.0`                   | v1.0.0-beta1      |


## Cache & Push
Prometheus is designed to scrape metrics. During each scrape the mongodb-query-exporter will evaluate all configured metrics.
If you have expensive queries there is an option to cache the aggregation result by setting a cache ttl.
However it is more effective to **avoid cache** and design good aggregation pipelines. In some cases a different scrape interval might also be a solution.
For individual aggregations and/or MongoDB servers older than 3.6 it might still be a good option though.

A better approach is using push instead a static cache, see bellow.

Example:
```yaml
aggregations:
- metrics:
  - name: myapp_example_simplevalue_total
    help: 'Simple gauge metric which is cached for 5min'
    value: total
  servers: [main]
  mode: pull
  cache: 5m
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
aggregations:
- metrics:
  - name: myapp_example_simplevalue_total
    help: 'Simple gauge metric'
    value: total
  servers: [main]
  # With the mode push the pipeline is only executed if a change occured on the collection called objects
  mode: push
  database: mydb
  collection: objects
  pipeline: |
    [
      {"$count":"total"}
    ]
```

## Debug
The mongodb-query-exporters also publishes a counter metric called `mongodb_query_exporter_query_total` which counts query results for each configured aggregation.
Furthermore you might increase the log level to get more insight.

## Used by
* The balloon helm chart implements the mongodb-query-exporter to expose general stats from the MongoDB like the number of total nodes or files stored internally or externally.
See the [config-map here](https://github.com/gyselroth/balloon-helm/blob/master/unstable/balloon/charts/balloon-mongodb-metrics/templates/config-map.yaml).


Please submit a PR if your project should be listed here!
