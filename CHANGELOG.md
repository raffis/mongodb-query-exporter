# 1.0.0
**Date**: 2022-03-18

Out of beta!
Notably the config format v3.0 was introduced in this release. See readme.md for more information.

## Features
* Support for env substitution within mongodb uri
* Support multiple metric values per aggregation pipeline #24

## Changes
* update: mongodb driver 1.8.4 #38 @fredmaggiowski
* Log a warning if no metrics are configured
* Upgrade to go1.17
* Add matrix mongodb version integration tests
* Various code improvements

## Bugfixes
* Exporter works now even without any servers configured in the config and falls back to the uri env or localhost
* Fallback to default logger if an empty encoder and/or level is configured

## Packaging
* Add support for podMonitor in helm chart
* kustomize base #29
* Add e2e tests and improved pipeline
* Add matrix mongodb version e2e tests


# 1.0.0-beta8
**Date**: 2021-11-04

## Features
* Add an option to override value for empty result set #32 @guillaumelecerf

## Changes
* Helper functions for shared chart labels #33


# 1.0.0-beta7
**Date**: 2021-01-18

## Bugfixes
* Fixes missing Path in ServiceMonitor helm chart

## Features
* Support changeable metrics path #19


# 1.0.0-beta6
**Date**: 2020-11-12

## Bugfixes
* Protect cache from concurrent access

## Changes
* User default prometheus registry instead a custom on


# 1.0.0-beta5
**Date**: 2020-11-11

## Features
* Support for multiple MongoDB servers
* Support for versioned configurations
* Redesigned GO public API
* New log implementation, the default is now json log format
* Added new counter metric for internal stats about quries `mongodb_query_exporter_query_total`
* Added root hint to go to /metrics

## Changes
* Dropped interval pulling mechanism, pull is now synchronous while getting called by an http request
* Drop support for counters (use gauge instead)
* No internal tracking of upstreams anymore, using prometheus ConstMetric
* go 1.15 update

## Packaging
* Provide Helm chart
* Migrated to github actions (from travis-ci)


# 1.0.0-beta4
**Date**: 2020-03-24

## Bugfixes
* Do not abort if lookup config in home fails
* Do not abort pull listeners if an error occurs #2
* Do not panic if MongoDB is unreachable during bootstrap

## Changes
* Execute docker container rootless by default
* introduce two config changes: cacheTime => interval / realtime => mod [pull|push]

## Packaging
* Do not build on travis and in Dockerfile, only build docker image on travis which contains everything else


# 1.0.0-beta3
**Date**: 2020-03-13

## Bugfixes
* Metrics never updated within startPullListeners? #1
* Proper usage of defaultCacheTime from mongodb.defaultCacheTime
* fixes logging output during testing

## Changes
* go 1.13 update


## Packaging
* correctly push build to github releases from travis
