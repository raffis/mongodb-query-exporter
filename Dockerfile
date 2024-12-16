FROM gcr.io/distroless/static:nonroot@sha256:6cd937e9155bdfd805d1b94e037f9d6a899603306030936a3b11680af0c2ed58
WORKDIR /
COPY mongodb-query-exporter mongodb-query-exporter
EXPOSE      9412

ENTRYPOINT ["/mongodb-query-exporter"]
