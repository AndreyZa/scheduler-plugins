SHELL := /bin/bash

PLUGIN_DIR ?= pkg/sensitivityscore
PLUGIN_PKG ?= ./$(PLUGIN_DIR)/...

DEV_REGISTRY    ?= andreyza/sensitivityscore
DEV_VERSION     ?= v$(shell date +%Y%m%d)-$(shell git describe --tags --match "v*")
DEV_IMAGE       ?= localhost:5000/scheduler-plugins/sensitivityscore:$(DEV_VERSION)
K8S_VERSION     ?= v1.35.0
DEV_GOARCH      ?= $(shell go env GOARCH)

.PHONY: help
help:
	@echo "sensitivityscore.mk (только сборка) — доступные команды:"
	@grep -h -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| grep -E '^ss-' \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## gofmt -w по пакету плагина
	gofmt -l -w $(PLUGIN_DIR)

.PHONY: vet
vet: ## go vet по пакету плагина
	go vet $(PLUGIN_PKG)

.PHONY: build-ss
build-ss: fmt vet ## Собрать пакет плагина локально (без Docker-образа) — быстрая проверка компиляции
	go build $(PLUGIN_PKG)

.PHONY: test
test: ## Юнит-тесты плагина
	go test -v -count=1 $(PLUGIN_PKG)

.PHONY: dev-image
dev-image:
	GOOS=linux GOARCH=$(DEV_GOARCH) go build \
		-ldflags '-X k8s.io/component-base/version.gitVersion=$(K8S_VERSION) -w' \
		-o bin/kube-scheduler cmd/scheduler/main.go
	docker build --no-cache -t $(DEV_IMAGE) -f Dockerfile.dev .

.PHONY: dev-push
dev-push:
	docker tag $(DEV_IMAGE) $(DEV_REGISTRY):$(DEV_VERSION)
	docker push $(DEV_REGISTRY):$(DEV_VERSION)

.PHONY: dev-release
dev-release: dev-image dev-push
