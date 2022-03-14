FROM golang:1.15 as builder

ADD .   /go/src/github.com/raffis/mongodb-query-exporter
WORKDIR /go/src/github.com/raffis/mongodb-query-exporter


RUN make deps build

FROM gcr.io/distroless/base
COPY --from=builder /go/src/github.com/raffis/mongodb-query-exporter/mongodb_query_exporter /bin/mongodb_query_exporter

ENV MDBEXPORTER_CONFIG /etc/mongodb-query-exporter/config.yaml
USER 1000:1000

EXPOSE      9412
ENTRYPOINT [ "/bin/mongodb_query_exporter" ]
