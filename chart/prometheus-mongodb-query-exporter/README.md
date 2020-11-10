# Prometheus MongoDB Exporter

Installs the [MongoDB Query Exporter](https://github.com/raffis/mongodb-query-exporter) for [Prometheus](https://prometheus.io/).

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm upgrade --install my-release mongodb-query-exporter/prometheus-mongodb-exporter --set mongodb.0 mongodb://mymongodb:27017
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
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `affinity` | Node/pod affinities | `{}` |
| `annotations` | Annotations to be added to the pods | `{}` |
| `config` | The configuration for the mongodb-query-exporter. See README.md or the examples directory for examples. | `` |
| `existingConfig.name` | Refer to an existing configmap name instead of using `config` | `` |
| `existingSecret.name` | Refer to an existing secret name instead of using a list `mongodb` | `` |
| `extraArgs` | The extra command line arguments to pass to the MongoDB Exporter  | See values.yaml |
| `fullnameOverride` | Override the full chart name | `` |
| `image.pullPolicy` | MongoDB Exporter image pull policy | `IfNotPresent` |
| `image.repository` | MongoDB Exporter image name | `raffis/mongodb-query-exporter` |
| `image.tag` | MongoDB query Exporter image tag | `v1.0.0-beta5` |
| `imagePullSecrets` | List of container registry secrets | `[]` |
| `mongodb` | A list of [URI](https://docs.mongodb.com/manual/reference/connection-string) to connect to MongoDB. These will be used as connection URI in the query exporter config. You don't need to reference the MongoDB server in the config. | `[]` |
| `nameOverride` | Override the application name  | `` |
| `nodeSelector` | Node labels for pod assignment | `{}` |
| `podAnnotations` | Annotations to be added to all pods | `{}` |
| `port` | The container port to listen on | `9216` |
| `priorityClassName` | Pod priority class name | `` |
| `replicas` | Number of replicas in the replica set | `1` |
| `resources` | Pod resource requests and limits | `{}` |
| `env` | Extra environment variables passed to pod | `{}` |
| `securityContext` | Security context for the pod | See values.yaml |
| `service.labels` | Additional labels for the service definition | `{}` |
| `service.annotations` | Annotations to be added to the service | `{}` |
| `service.port` | The port to expose | `9216` |
| `service.type` | The type of service to expose | `ClusterIP` |
| `serviceAccount.create` | If `true`, create the service account | `true` |
| `serviceAccount.name` | Name of the service account | `` |
| `serviceMonitor.enabled` | Set to true if using the Prometheus Operator | `true` |
| `serviceMonitor.interval` | Interval at which metrics should be scraped | `30s` |
| `serviceMonitor.scrapeTimeout` | Interval at which metric scrapes should time out | `10s` |
| `serviceMonitor.namespace` | The namespace where the Prometheus Operator is deployed | `` |
| `serviceMonitor.additionalLabels` | Additional labels to add to the ServiceMonitor | `{}` |
| `serviceMonitor.targetLabels` | Set of labels to transfer on the Kubernetes Service onto the target. | `[]`
| `serviceMonitor.metricRelabelings` | MetricRelabelConfigs to apply to samples before ingestion. | `[]` |
| `tolerations` | List of node taints to tolerate  | `[]` |
