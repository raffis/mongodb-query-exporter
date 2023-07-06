FROM gcr.io/distroless/static:nonroot@sha256:9ecc53c269509f63c69a266168e4a687c7eb8c0cfd753bd8bfcaa4f58a90876f
WORKDIR /
COPY mongodb-query-exporter mongodb-query-exporter
EXPOSE      9412

ENTRYPOINT ["/mongodb-query-exporter"]
