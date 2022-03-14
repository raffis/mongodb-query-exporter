# 1.0.0
**Date**: 2022-01-24

Out of beta!

## Packaging
* Add support for podMonitor in helm chart
* kustomize base
* Add e2e tests and improved pipeline


# 1.0.0-beta7
**Date**: Mon Jan 18 21:34:20 CET 2020

## Bugfixes
* Fixes missing Path in ServiceMonitor helm chart

## Features
* Support changeable metrics path #19


# 1.0.0-beta6
**Date**: Thu Nov 12 22:35:21 CET 2020

## Bugfixes
* Protect cache from concurrent access

## Changes
* User default prometheus registry instead a custom on


# 1.0.0-beta5
**Date**: Wed Nov 11 22:33:21 CET 2020

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
**Date**: Tue Mar 24 15:57:21 CET 2020

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
**Date**: Fri Mar 13 16:33:22 CET 2020

## Bugfixes
* Metrics never updated within startPullListeners? #1
* Proper usage of defaultCacheTime from mongodb.defaultCacheTime
* fixes logging output during testing

## Changes
* go 1.13 update


## Packaging
* correctly push build to github releases from travis
