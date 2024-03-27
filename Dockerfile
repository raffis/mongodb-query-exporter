FROM gcr.io/distroless/static:nonroot@sha256:6732c3975d97fac664a5ed15a81a5915e023a7b5a7b58195e733c60b8dc7e684
WORKDIR /
COPY mongodb-query-exporter mongodb-query-exporter
EXPOSE      9412

ENTRYPOINT ["/mongodb-query-exporter"]
