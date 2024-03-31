# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

BUILD_DATE                  := $(shell date '+%Y-%m-%dT%H:%M:%S%z' | sed 's/\([0-9][0-9]\)$$/:\1/g')
NAME                        := gardener-custom-metrics
IMAGE_REGISTRY_URI          := europe-docker.pkg.dev/gardener-project/releases/gardener
REPO_ROOT                   := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
VERSION                     := $(shell cat "$(REPO_ROOT)/VERSION")
EFFECTIVE_VERSION           := $(VERSION)-$(shell git rev-parse HEAD)
LEADER_ELECTION             := false

ifneq ($(strip $(shell git status --porcelain 2>/dev/null)),)
	EFFECTIVE_VERSION := $(EFFECTIVE_VERSION)-dirty
endif

# In debug, do not use the -w flag. It strips useful debug information.
LD_FLAGS := "-w $(shell EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) $(REPO_ROOT)/third_party/gardener/gardener/hack/get-build-ld-flags.sh k8s.io/component-base $(REPO_ROOT)/VERSION $(NAME))"
LD_FLAGS_DEBUG := "$(shell EFFECTIVE_VERSION=$(EFFECTIVE_VERSION) $(REPO_ROOT)/third_party/gardener/gardener/hack/get-build-ld-flags.sh k8s.io/component-base $(REPO_ROOT)/VERSION $(NAME))"

TOOLS_DIR := $(REPO_ROOT)/hack/tools
include $(REPO_ROOT)/third_party/gardener/gardener/hack/tools.mk

#########################################
# Rules for local development scenarios #
#########################################

.PHONY: start
start:
	@LEADER_ELECTION_NAMESPACE=garden GO111MODULE=on go run \
		-ldflags $(LD_FLAGS) \
		./cmd/gardener-custom-metrics \
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
	@LD_FLAGS=$(LD_FLAGS) $(REPO_ROOT)/third_party/gardener/gardener/hack/install.sh ./...

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

.PHONY: tidy
tidy:
	@GO111MODULE=on go mod tidy

.PHONY: clean
clean:
	@$(REPO_ROOT)/third_party/gardener/gardener/hack/clean.sh ./cmd/... ./pkg/...

.PHONY: check-generate
check-generate:
	echo "Code generation is currently not implemented"
	# @$(REPO_ROOT)/third_party/gardener/gardener/hack/check-generate.sh $(REPO_ROOT)

.PHONY: check
check: $(GOIMPORTS) $(GOLANGCI_LINT) $(HELM)
	@$(REPO_ROOT)/third_party/gardener/gardener/hack/check.sh --golangci-lint-config=./.golangci.yaml ./cmd/... ./pkg/...

.PHONY: generate
generate: $(CONTROLLER_GEN) $(GEN_CRD_API_REFERENCE_DOCS) $(HELM) $(YQ)
	echo "Code generation is currently not implemented"
	# @$(REPO_ROOT)/third_party/gardener/gardener/hack/generate.sh ./cmd/... ./pkg/...
	# $(MAKE) format

.PHONY: format
format: $(GOIMPORTS) $(GOIMPORTSREVISER)
	@$(REPO_ROOT)/third_party/gardener/gardener/hack/format.sh ./cmd ./pkg

.PHONY: test
test: $(REPORT_COLLECTOR)
	@$(REPO_ROOT)/third_party/gardener/gardener/hack/test.sh ./cmd/... ./pkg/...

.PHONY: test-cov
test-cov:
	@$(REPO_ROOT)/third_party/gardener/gardener/hack/test-cover.sh ./cmd/... ./pkg/...

.PHONY: test-clean
test-clean:
	@$(REPO_ROOT)/third_party/gardener/gardener/hack/test-cover-clean.sh

.PHONY: verify
verify: check format test

.PHONY: verify-extended
verify-extended: check-generate check format test-cov test-clean

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
