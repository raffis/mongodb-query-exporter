FROM golang:1.20 as builder

ADD .   /go/src/github.com/raffis/mongodb-query-exporter
WORKDIR /go/src/github.com/raffis/mongodb-query-exporter


RUN make deps build

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /go/src/github.com/raffis/mongodb-query-exporter/mongodb-query-exporter /bin/mongodb-query-exporter
USER 1000:1000

EXPOSE      9412
ENTRYPOINT [ "/bin/mongodb-query-exporter" ]
