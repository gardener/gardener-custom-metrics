# gardener-custom-metrics

[![REUSE status](https://api.reuse.software/badge/github.com/gardener/gardener-custom-metrics)](https://api.reuse.software/info/github.com/gardener/gardener-custom-metrics)

## Overview

The `gardener-custom-metrics` component operates as a K8s API service, adding functionality to the seed kube-apiserver. It periodically scrapes the metrics endpoints of all shoot kube-apiserver pods on the seed. It implements the K8s custom metrics API and provides K8s metrics specific to Gardener, based on custom calculations.

