# Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

BUILD_DATE := $(shell date '+%Y-%m-%dT%H:%M:%S%z' | sed 's/\([0-9][0-9]\)$$/:\1/g')
NAME                        := gardener-custom-metrics
IMAGE_REGISTRY_URI          := eu.gcr.io/gardener-project/gardener
REPO_ROOT                   := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
VERSION                     := $(shell cat "$(REPO_ROOT)/VERSION")
EFFECTIVE_VERSION           := $(VERSION)-$(shell git rev-parse HEAD)
LD_FLAGS                    := "-w -X github.com/gardener/$(NAME)/pkg/version.Version=$(VERSION) -X github.com/gardener/$(NAME)/pkg/version.GitCommit=$(shell git rev-parse --verify HEAD) -X github.com/gardener/$(NAME)/pkg/version.BuildDate=$(shell date --rfc-3339=seconds | sed 's/ /T/')"
LEADER_ELECTION             := false
PARALLEL_E2E_TESTS          := 2

ifndef ARTIFACTS
	export ARTIFACTS=/tmp/artifacts
endif

ifneq ($(strip $(shell git status --porcelain 2>/dev/null)),)
	EFFECTIVE_VERSION := $(EFFECTIVE_VERSION)-dirty
endif

# In debug, do not use the -w flag. It strips useful debug information.
LD_FLAGS := "-w $(shell EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) $(REPO_ROOT)/hack/gardener-util/get-build-ld-flags.sh k8s.io/component-base $(REPO_ROOT)/VERSION $(EXTENSION_PREFIX)-$(NAME))"
LD_FLAGS_DEBUG := "$(shell EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) $(REPO_ROOT)/hack/gardener-util/get-build-ld-flags.sh k8s.io/component-base $(REPO_ROOT)/VERSION $(EXTENSION_PREFIX)-$(NAME))"

TOOLS_DIR := $(REPO_ROOT)/hack/gardener-util/tools
include $(REPO_ROOT)/hack/gardener-util/tools.mk

#########################################
# Rules for local development scenarios #
#########################################

.PHONY: start
start:
	@LEADER_ELECTION_NAMESPACE=garden GO111MODULE=on go run \
		-mod=vendor \
		-ldflags $(LD_FLAGS) \
		./cmd \
		--secure-port=6443 \
		--lister-kubeconfig=$(REPO_ROOT)/dev/kubeconfig.yaml \
		--authentication-kubeconfig=$(REPO_ROOT)/dev/kubeconfig.yaml \
		--authorization-kubeconfig=$(REPO_ROOT)/dev/kubeconfig.yaml \
		--tls-cert-file=$(REPO_ROOT)/example/serving-cert/serving-cert.crt \
		--tls-private-key-file=$(REPO_ROOT)/example/serving-cert/serving-cert.key \
		--leader-election=false \
		--kubeconfig=$(REPO_ROOT)/dev/kubeconfig.yaml \
		--namespace=garden \
		--access-ip=10.223.130.159 \
		--access-port=9443 \
		--debug \
		--log-level 75

#################################################################
# Rules related to binary build, Docker image build and release #
#################################################################

# Installs the binary. Used by docker build
.PHONY: install
install:
	@LD_FLAGS=$(LD_FLAGS) $(REPO_ROOT)/hack/gardener-util/install.sh ./...

.PHONY: docker-login
docker-login:
	@gcloud auth activate-service-account --key-file .secrets/gcr-readwrite.json

# Build the docker image
.PHONY: docker-build
docker-build:
	@docker build -t $(IMAGE_REGISTRY_URI)/$(NAME):$(VERSION) -t $(IMAGE_REGISTRY_URI)/$(NAME):latest -f Dockerfile -m 6g --target $(NAME) .

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push $(IMAGE_REGISTRY_URI)/$(NAME):$(VERSION)
	docker push $(IMAGE_REGISTRY_URI)/$(NAME):latest

#####################################################################
# Rules for verification, formatting, linting, testing and cleaning #
#####################################################################

.PHONY: revendor
revendor:
	@GO111MODULE=on go mod tidy
	@GO111MODULE=on go mod vendor

.PHONY: clean
clean:
	@$(REPO_ROOT)/hack/gardener-util/clean.sh ./cmd/... ./pkg/... ./test/...

.PHONY: check-generate
check-generate:
	echo "Code generation is currently not implemented"
	# @$(REPO_ROOT)/hack/gardener-util/check-generate.sh $(REPO_ROOT)

.PHONY: check-docforge
check-docforge: $(DOCFORGE)
	@$(REPO_ROOT)/hack/gardener-util/check-docforge.sh $(REPO_ROOT) $(REPO_ROOT)/.docforge/manifest.yaml ".docforge/;docs/" $(NAME) false

.PHONY: check
check: $(GOIMPORTS) $(GOLANGCI_LINT) $(HELM)
	@$(REPO_ROOT)/hack/gardener-util/check.sh --golangci-lint-config=./.golangci.yaml ./cmd/... ./pkg/...

.PHONY: generate
generate: $(CONTROLLER_GEN) $(GEN_CRD_API_REFERENCE_DOCS) $(HELM) $(YQ)
	echo "Code generation is currently not implemented"
	# @$(REPO_ROOT)/hack/gardener-util/generate.sh ./cmd/... ./pkg/... ./test/...
	# $(MAKE) format

.PHONY: format
format: $(GOIMPORTS) $(GOIMPORTSREVISER)
	@$(REPO_ROOT)/hack/gardener-util/format.sh ./cmd ./pkg ./test

.PHONY: test
test: $(REPORT_COLLECTOR)
	@$(REPO_ROOT)/hack/gardener-util/test.sh ./cmd/... ./pkg/...

.PHONY: test-cov
test-cov:
	@$(REPO_ROOT)/hack/gardener-util/test-cover.sh ./cmd/... ./pkg/...

.PHONY: test-clean
test-clean:
	@$(REPO_ROOT)/hack/gardener-util/test-cover-clean.sh

.PHONY: verify
verify: check check-docforge format test

.PHONY: verify-extended
verify-extended: check-generate check check-docforge format test test-cov test-clean

# skaffold dev and debug clean up deployed modules by default, disable this
debug: export SKAFFOLD_CLEANUP = false
# Artifacts might be already built when you decide to start debugging.
# However, these artifacts do not include the gcflags which `skaffold debug` sets automatically, so delve would not work.
# Disabling the skaffold cache for debugging ensures that you run artifacts with gcflags required for debugging.
debug: export SKAFFOLD_CACHE_ARTIFACTS = false

debug: export SOURCE_DATE_EPOCH = $(shell date -d $(BUILD_DATE) +%s)

.PHONY: debug
debug: $(SKAFFOLD)
	@LD_FLAGS=$(LD_FLAGS_DEBUG) $(SKAFFOLD) debug

	# TODO: Andrey: P1: Inject TLS secret name dynamically into deployment
	# GCMX_TLS_SECRET_NAME=$(kubectl -n garden get secrets | grep '^gardener-custom-metrics' | head -n 1 | awk '{print $1}') \
    # TODO: Andrey: P1: code cleanup
	# export SKAFFOLD_DEFAULT_REPO = localhost:5001
	# export SKAFFOLD_PUSH = true

	# skaffold dev triggers new builds and deployments immediately on file changes by default,
	# this is too heavy in a large project like gardener, so trigger new builds and deployments manually instead.
	# gardener%dev gardenlet%dev operator-dev: export SKAFFOLD_TRIGGER = manual
