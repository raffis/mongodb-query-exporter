FROM quay.io/prometheus/golang-builder AS builder

ADD .   /go/src/github.com/raffis/mongodb-query-exporter
WORKDIR /go/src/github.com/raffis/mongodb-query-exporter

RUN make

FROM        quay.io/prometheus/busybox:glibc
COPY        --from=builder /go/src/github.com/raffis/mongodb-query-exporter/mongodb_query_exporter /bin/mongodb_query_exporter

EXPOSE      9412
ENTRYPOINT [ "/bin/mongodb_query_exporter" ]
