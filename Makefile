# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)

VERSION ?= 1.24.0

COMMIT := $(shell git rev-parse --short HEAD)
DATE := $(shell date +%Y%m%d)
VERSION := $(VERSION)-dev.$(COMMIT)-$(DATE)

# TODO For daily pushes, create dev channel (k8ssandra bundle, not datastax) - or set these in the
# .github
# Or add --package and define k8ssandra/cass-operator and datastax/cass-operator? 
DEFAULT_CHANNEL ?= "stable" 
CHANNELS ?= "stable"

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "preview,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=preview,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="preview,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# cassandra.datastax.com/cass-oper-bundle:$VERSION and cassandra.datastax.com/cass-oper-catalog:$VERSION.

REGISTRY ?=
ORG ?= k8ssandra
IMAGE_TAG_BASE ?= $(ORG)/cass-operator

M_INTEG_DIR ?= all

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):v$(VERSION)
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:generateEmbeddedObjectMeta=true"
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.30.x

# Logger image
LOG_IMG_BASE ?= $(ORG)/system-logger
LOG_IMG ?= $(LOG_IMG_BASE):v$(VERSION)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

BUILDX_OPTIONS ?=

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code.
	$(GOLANGCI_LINT) run ./...

.PHONY: test
test: manifests generate fmt vet lint envtest ## Run tests.
	# Old unit tests first - these use mocked client / fakeclient
	go test ./pkg/... -coverprofile cover-pkg.out
	# Then the envtest ones
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -v ./apis/... ./internal/controllers/... -coverprofile cover.out

.PHONY: integ-test
integ-test: kustomize cert-manager helm ## Run integration tests from directory M_INTEG_DIR or set M_INTEG_DIR=all to run all the integration tests.
ifeq ($(M_INTEG_DIR), all)
	# Run all the tests (exclude kustomize & testdata directories)
	cd tests && go test -v ./... -timeout 60m --ginkgo.show-node-events --ginkgo.v
else
	cd tests/${M_INTEG_DIR} && go test -v ./... -timeout 60m --ginkgo.show-node-events --ginkgo.v
endif

.PHONY: version
version:
	@echo $(VERSION)

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker buildx build --build-arg VERSION=${VERSION} -t ${IMG} ${BUILDX_OPTIONS} . --load

.PHONY: docker-kind
docker-kind: docker-build ## Build docker image and load to kind cluster
	kind load docker-image ${IMG}

.PHONY: docker-push
docker-push: ## Build and push docker image with the manager.
	docker push ${IMG}

.PHONY: docker-logger-build
docker-logger-build: ## Build system-logger image.
	docker buildx build -t ${LOG_IMG} --build-arg VERSION=${VERSION} -f logger.Dockerfile . --load

.PHONY: docker-logger-push
docker-logger-push: ## Push system-logger-image
	docker push ${LOG_IMG}

.PHONY: docker-logger-kind
docker-logger-kind: docker-logger-build ## Build system-logger image and load to kind cluster
	kind load docker-image ${LOG_IMG}

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	kubectl apply --force-conflicts --server-side -k config/crd

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kubectl delete --ignore-not-found=$(ignore-not-found) -k config/crd

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	LOG_IMG=${LOG_IMG} yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml
	kubectl apply --force-conflicts --server-side -k config/deployments/default

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	kubectl delete --ignore-not-found=$(ignore-not-found) -k config/deployments/default

.PHONY: deploy-test
deploy-test: kustomize
ifneq ($(strip $(NAMESPACE)),)
	cd tests/kustomize && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
endif
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	LOG_IMG=${LOG_IMG} yq eval -i '.images.system-logger = env(LOG_IMG)' config/manager/image_config.yaml
	kubectl apply --force-conflicts --server-side -k tests/$(TEST_DIR)

.PHONY: undeploy-test
undeploy-test: kustomize
ifneq ($(strip $(NAMESPACE)),)
	cd tests/kustomize && $(KUSTOMIZE) edit set namespace $(NAMESPACE)
endif
	kubectl delete -k tests/$(TEST_DIR)

##@ Tools / Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
HELM ?= $(LOCALBIN)/helm
OPM ?= $(LOCALBIN)/opm

## Tool Versions
CERT_MANAGER_VERSION ?= v1.14.7
KUSTOMIZE_VERSION ?= v5.5.0
CONTROLLER_TOOLS_VERSION ?= v0.15.0
OPERATOR_SDK_VERSION ?= 1.35.0
HELM_VERSION ?= 3.14.2
OPM_VERSION ?= 1.38.0
GOLINT_VERSION ?= 1.62.2

.PHONY: cert-manager
cert-manager: ## Install cert-manager to the cluster
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/$(CERT_MANAGER_VERSION)/cert-manager.yaml
	kubectl wait --for=condition=Established crd certificates.cert-manager.io
	kubectl rollout status deployment cert-manager-webhook -n cert-manager

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
ifeq ($(GITHUB_ACTIONS), true)
	@echo "Running in GitHub Actions, using the kustomize version provided by the runner."
	test -e $(LOCALBIN)/kustomize || ln -s $(shell which kustomize) $(LOCALBIN)/kustomize
else
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || { curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }
endif

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(ENVTEST) || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

OS=$(shell go env GOOS)
ARCH=$(shell go env GOARCH)

HELMTARNAME = helm-v$(HELM_VERSION)-${OS}-${ARCH}.tar.gz
.PHONY: helm
helm: ## Download helm locally if necessary.
ifeq (,$(wildcard $(HELM)))
ifeq (,$(shell which helm 2>/dev/null))
	@{ \
	set -e ;\
	# mkdir -p $(dir $(HELM)) ;\
	curl -sSLo bin/${HELMTARNAME} https://get.helm.sh/${HELMTARNAME} ;\
	tar -zxf bin/${HELMTARNAME} -C bin/;\
	mv bin/${OS}-${ARCH}/helm ${HELM} ;\
	rm -rf bin/${HELMTARNAME} bin/${OS}-${ARCH} ;\
	chmod +x $(HELM) ;\
	}
else
HELM = $(shell which helm)
endif
endif

.PHONY: operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/v$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests --interactive=false -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build --load-restrictor LoadRestrictionsNone config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(BUNDLE_GEN_FLAGS)
	scripts/postprocess-bundle.sh $(REGISTRY)
	$(OPERATOR_SDK) bundle validate ./bundle --select-optional suite=operatorframework

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker buildx build -f bundle.Dockerfile -t $(BUNDLE_IMG) . --load

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	docker push $(BUNDLE_IMG)

.PHONY: opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v$(OPM_VERSION)/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	docker push $(CATALOG_IMG)

.PHONY: golangci-lint
golangci-lint:
	@if test -x $(LOCALBIN)/golangci-lint && ! $(LOCALBIN)/golangci-lint version | grep -q $(GOLINT_VERSION); then \
		echo "$(LOCALBIN)/golangci-lint version is not expected $(GOLINT_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/golangci-lint; \
	fi
	test -s $(LOCALBIN)/golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v$(GOLINT_VERSION)
