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


all: deps vet format build test

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

deps:
	@echo ">> install dependencies"
	@$(GO) mod download

format:
	@echo ">> formatting code"
	@$(GO) fmt $(pkgs)

vet:
	@echo ">> vetting code"
	@$(GO) vet $(pkgs)

build:
	@echo ">> building binaries"
	go build -o mongodb_query_exporter main.go

.PHONY: run
run: manifests generate fmt vet
	go run ./main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: deploy
deploy:
	cd deploy/exporter && $(KUSTOMIZE) edit set image exporter=${IMG}
	$(KUSTOMIZE) build deploy/exporter | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy exporter from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy-test
deploy-test:
	cd deploy/test && $(KUSTOMIZE) edit set image exporter=${IMG}
	$(KUSTOMIZE) build deploy/test | kubectl apply -f -

.PHONY: undeploy-test
undeploy-test: ## Undeploy exporter from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/test | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: all style format build test vet
