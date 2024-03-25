# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

############# builder
FROM golang:1.22.1 AS builder

WORKDIR /go/src/github.com/gardener/gardener-custom-metrics

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN make install

############# gardener-custom-metrics
FROM gcr.io/distroless/static-debian12:nonroot AS gardener-custom-metrics
WORKDIR /

COPY --from=builder /go/bin/gardener-custom-metrics /gardener-custom-metrics
ENTRYPOINT ["/gardener-custom-metrics"]
