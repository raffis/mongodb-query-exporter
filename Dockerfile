FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY mongodb-query-exporter mongodb-query-exporter
EXPOSE      9412

ENTRYPOINT ["/mongodb-query-exporter"]
