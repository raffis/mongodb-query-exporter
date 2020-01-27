FROM quay.io/prometheus/golang-builder AS builder

ADD .   /go/src/github.com/raffis/mongodb-query-exporter
WORKDIR /go/src/github.com/raffis/mongodb-query-exporter

RUN make

FROM        quay.io/prometheus/busybox:glibc
MAINTAINER  Raffael Sahli <public@raffaelsahli.com>
COPY        --from=builder /go/src/github.com/raffis/prom-mongodb-query-exporter /bin/mongodb_query_exporter

EXPOSE      9399
ENTRYPOINT [ "/bin/mongodb_query_exporter" ]
