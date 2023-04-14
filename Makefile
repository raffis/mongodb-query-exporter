# Copyright 2015 The Prometheus Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

GO     := go
GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
KUSTOMIZE := kustomize
IMG := raffis/mongodb-query-exporter:latest
pkgs    := $(shell $(GO) list ./... | grep -v /vendor/)
units    := $(shell $(GO) list ./... | grep -v /vendor/ | grep -v cmd)
integrations    := $(shell $(GO) list ./... | grep cmd)

PREFIX              ?= $(shell pwd)
BIN_DIR             ?= $(shell pwd)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: deps vet fmt lint test build

style:
	@echo ">> checking code style"
	@! gofmt -d $(shell find . -path ./vendor -prune -o -name '*.go' -print) | grep '^'

test: unittest integrationtest

unittest:
	@echo ">> running unit tests"
	@$(GO) test -short -race -v -coverprofile=coverage.out $(units)

integrationtest:
	@echo ">> running integration tests"
	@$(GO) test -short -race -v  $(integrations)

GOLANGCI_LINT = $(GOBIN)/golangci-lint
.PHONY: golangci-lint
golangci-lint: ## Download golint locally if necessary
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint@v1.52.2)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against code
	$(GOLANGCI_LINT) run ./...

deps:
	@echo ">> install dependencies"
	@$(GO) mod download

fmt:
	@echo ">> formatting code"
	@$(GO) fmt $(pkgs)

vet:
	@echo ">> vetting code"
	@$(GO) vet $(pkgs)

build:
	@echo ">> building binaries"
	go build -o mongodb-query-exporter cmd/main.go

.PHONY: run
run: fmt vet
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: deploy
deploy:
	cd deploy/exporter && $(KUSTOMIZE) edit set image ghcr.io/raffis/mongodb-query-exporter=${IMG}
	$(KUSTOMIZE) build deploy/exporter | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy exporter from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy-test
deploy-test:
	cd deploy/test && $(KUSTOMIZE) edit set image ghcr.io/raffis/mongodb-query-exporter=${IMG}
	$(KUSTOMIZE) build deploy/test | kubectl apply -f -

.PHONY: undeploy-test
undeploy-test: ## Undeploy exporter from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/test | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: all style fmt build test vet

# go-install-tool will 'go install' any package $2 and install it to $1
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
env -i bash -c "GOBIN=$(GOBIN) PATH=$(PATH) GOPATH=$(shell go env GOPATH) GOCACHE=$(shell go env GOCACHE) go install $(2)" ;\
rm -rf $$TMP_DIR ;\
}
endef
