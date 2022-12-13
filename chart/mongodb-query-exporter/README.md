# MongoDB Exporter

Installs the [MongoDB Query Exporter](https://github.com/raffis/mongodb-query-exporter) for [Prometheus](https://prometheus.io/).

## Installing the Chart

To install the chart with the release name `mongodb-query-exporter`:

```console
helm upgrade mongodb-query-exporter --install oci://ghcr.io/raffis/charts/mongodb-query-exporter --set mongodb[0]=mongodb://mymongodb:27017 --set-file config=../../example/configv2.yaml
```

This command deploys the MongoDB Exporter with the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Using the Chart

To use the chart, please add your MongoDB server to the list of servers you want to query `mongodb` and ensure it is populated with a valid [MongoDB URI](https://docs.mongodb.com/manual/reference/connection-string).
You may add multiple ones if you want to query more than one MongoDB server.
Or an existing secret (in the releases namespace) with MongoDB URI's referred via `existingSecret.name`.
If the MongoDB server requires authentication, credentials should be populated in the connection string as well. The MongoDB query exporter supports
connecting to either a MongoDB replica set member, shard, or standalone instance.

The chart comes with a ServiceMonitor for use with the [Prometheus Operator](https://github.com/helm/charts/tree/master/stable/prometheus-operator).
If you're not using the Prometheus Operator, you can disable the ServiceMonitor by setting `serviceMonitor.enabled` to `false` and instead
populate the `podAnnotations` as below:

```yaml
podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "metrics"
  prometheus.io/path: "/metrics"
```

## Configuration

See Customizing the Chart Before Installing. To see all configurable options with detailed comments, visit the chart's values.yaml, or run the configuration command:

```sh
$ helm show values oci://ghcr.io/raffis/charts/mongodb-query-exporter
```
