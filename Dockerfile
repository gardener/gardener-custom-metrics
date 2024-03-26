############# builder
FROM golang:1.22.1 AS builder

WORKDIR /go/src/github.com/gardener/gardener-custom-metrics
COPY . .
RUN make install

############# gardener-custom-metrics
FROM gcr.io/distroless/static-debian12:nonroot AS gardener-custom-metrics
WORKDIR /

COPY --from=builder /go/bin/gardener-custom-metrics /gardener-custom-metrics
ENTRYPOINT ["/gardener-custom-metrics"]
