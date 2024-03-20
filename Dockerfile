############# builder
FROM golang:1.22.1 AS builder

WORKDIR /go/src/github.com/gardener/gardener-custom-metrics
COPY . .
RUN make install

############# base image # TODO: Andrey: P1: Move to distroless
FROM alpine:3.18.6 AS base

############# gardener-custom-metrics
FROM base AS gardener-custom-metrics
WORKDIR /

COPY --from=builder /go/bin/gardener-custom-metrics /gardener-custom-metrics
ENTRYPOINT ["/gardener-custom-metrics"]
