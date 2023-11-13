############# builder
FROM golang:1.19.9 AS builder

WORKDIR /go/src/github.com/gardener/gardener-custom-metrics
COPY . .
RUN make install
# RUN CGO_ENABLED=0 GO111MODULE=on GOFLAGS=-mod=vendor go build -a -o gardener-custom-metrics.exe cmd/main.go

############# base image # TODO: Andrey: P1: Move to distroless
FROM alpine:3.18.0 AS base

############# gardener-custom-metrics
FROM base AS gardener-custom-metrics
WORKDIR /

COPY --from=builder /go/bin/gardener-custom-metrics /gardener-custom-metrics.exe
# COPY --from=builder /go/src/github.com/gardener/gardener-custom-metrics/gardener-custom-metrics.exe .
ENTRYPOINT ["/gardener-custom-metrics.exe"]
