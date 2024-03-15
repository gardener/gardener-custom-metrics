# gardener-custom-metrics
[![reuse compliant](https://reuse.software/badge/reuse-compliant.svg)](https://reuse.software/)
## Overview
The `gardener-custom-metrics` component operates as a K8s API service, adding functionality to the seed kube-apiserver.
It periodically scrapes the metrics endpoints of all shoot kube-apiserver pods on the seed. It implements the K8s custom
metrics API and provides K8s metrics specific to Gardener, based on custom calculations.
