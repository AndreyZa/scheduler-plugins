# sensitivityscore.mk — ТОЛЬКО сборка плагина SensitivityScore в этом форке.
#
# Разделение ответственности (сознательное решение):
#   - этот репозиторий (форк kubernetes-sigs/scheduler-plugins) — ТОЛЬКО
#     собирает Go-код плагина и производит Docker-образ. Больше ничего.
#   - весь деплой, ConfigMap-ы, харнесс экспериментов, анализ результатов —
#     живут в отдельном репозитории sensitivityscore-hpc-bench, и его
#     Makefile сам вызывает `make -C <путь-к-этому-форку> -f sensitivityscore.mk ss-image`,
#     когда нужен свежий образ.
#
# Контракт между репозиториями: результат `ss-image` — образ с тегом
# $(REGISTRY)/kube-scheduler:$(RELEASE_VERSION), ПОЛНОСТЬЮ детерминированным
# (не зависит от git describe/даты — по умолчанию апстримовский Makefile
# вычисляет тег как v<дата>-<git describe>, что ломается без тегов в репо и
# вообще плавает от прогона к прогону; здесь мы явно передаём оба значения
# через командную строку, которая перебивает даже target-specific
# присвоения в апстримовском Makefile — проверено эмпирически).
#
# Если меняешь REGISTRY/RELEASE_VERSION здесь — обнови и переменную
# SCHEDULER_IMAGE в Makefile репозитория sensitivityscore-hpc-bench, чтобы
# они продолжали ссылаться на один и тот же образ.
#
# Использование:
#   make -f sensitivityscore.mk ss-help
#   make -f sensitivityscore.mk ss-image
#
# Либо `include sensitivityscore.mk` в корневой Makefile (таргеты с
# префиксом ss- не пересекаются с родными build/clean этого репозитория).

SHELL := /bin/bash

PLUGIN_DIR ?= pkg/sensitivityscore
PLUGIN_PKG ?= ./$(PLUGIN_DIR)/...

# Реестр и тег итогового образа — единственный контракт с внешним
# потребителем (репозиторием sensitivityscore-hpc-bench).
REGISTRY        ?= localhost:5000/scheduler-plugins
RELEASE_VERSION ?= sensitivityscore-dev

.PHONY: ss-help
ss-help: ## Показать этот список команд
	@echo "sensitivityscore.mk (только сборка) — доступные команды:"
	@grep -h -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| grep -E '^ss-' \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: ss-fmt
ss-fmt: ## gofmt -w по пакету плагина
	gofmt -l -w $(PLUGIN_DIR)

.PHONY: ss-vet
ss-vet: ## go vet по пакету плагина
	go vet $(PLUGIN_PKG)

.PHONY: ss-build
ss-build: ss-fmt ss-vet ## Собрать пакет плагина локально (без Docker-образа) — быстрая проверка компиляции
	go build $(PLUGIN_PKG)

.PHONY: ss-test
ss-test: ## Юнит-тесты плагина
	go test -v -count=1 $(PLUGIN_PKG)

.PHONY: ss-image
ss-image: ss-build ## Собрать Docker-образ $(REGISTRY)/kube-scheduler:$(RELEASE_VERSION) — ЕДИНСТВЕННЫЙ результат этого репозитория для внешних потребителей
	$(MAKE) -f Makefile local-image REGISTRY=$(REGISTRY) RELEASE_VERSION=$(RELEASE_VERSION)

.PHONY: ss-clean
ss-clean: ## Убрать локальный build-кэш Go для пакета плагина
	go clean $(PLUGIN_PKG)
