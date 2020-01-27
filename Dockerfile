FROM quay.io/prometheus/golang-builder AS builder

ADD .   /go/src/github.com/raffis/prom-mongodb-query-exporter
WORKDIR /go/src/github.com/raffis/prom-mongodb-query-exporter

RUN make

FROM        quay.io/prometheus/busybox:glibc
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>
COPY        --from=builder /go/src/github.com/raffis/prom-mongodb-query-exporter /bin/mongodb_query_exporter

EXPOSE      9399
ENTRYPOINT [ "/bin/mongodb_query_exporter" ]
