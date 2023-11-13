#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# TODO: Andrey: P2: This is kept just for reference. Remove it.

set -o errexit
set -o nounset
set -o pipefail

echo "> E2E Tests"

ginkgo_flags=

seed_name="provider-extensions"

shoot_names=(
  e2e--gardener-custom-metrics
  # e2e-rslog-hib.local
)

# reduce flakiness in contended pipelines
export GOMEGA_DEFAULT_EVENTUALLY_TIMEOUT=5s
export GOMEGA_DEFAULT_EVENTUALLY_POLLING_INTERVAL=200ms
# if we're running low on resources, it might take longer for tested code to do something "wrong"
# poll for 5s to make sure, we're not missing any wrong action
export GOMEGA_DEFAULT_CONSISTENTLY_DURATION=5s
export GOMEGA_DEFAULT_CONSISTENTLY_POLLING_INTERVAL=200ms

GO111MODULE=on ginkgo run --timeout=1h $ginkgo_flags --v --show-node-events "$@"
