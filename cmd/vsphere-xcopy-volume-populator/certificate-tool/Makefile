# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CONFIG_FILE ?= config.yaml
ENV_FILE ?= .env

SHELL := /usr/bin/env bash

TARGET_NAMESPACE = $(shell yq e '.target-namespace' $(CONFIG_FILE))
STORAGE_VENDOR =$(shell yq e '.storage-vendor' $(CONFIG_FILE))
SECRET_NAME = $(shell yq e '.secret-name' $(CONFIG_FILE))
KUBECONFIG = $(shell yq e '.kubeconfig' $(CONFIG_FILE))

override TARGET_PVC ?= $(shell ( yq e '.target-pvc // ""' $(ENV_FILE) 2>/dev/null ))
override SOURCE_VMDK ?= $(shell ( yq e '.source-vmdk // ""' $(ENV_FILE) 2>/dev/null ))
override BACKEND ?= $(shell ( yq e '.backend // ""' $(ENV_FILE) 2>/dev/null ))
override PASSWORD ?= $(shell ( yq e '.password // ""' $(ENV_FILE) 2>/dev/null ))
override SERVICE_NAME ?= $(shell ( yq e '.serviceName // ""' $(ENV_FILE) 2>/dev/null ))
override SERVICE_PORT ?= $(shell ( yq e '.servicePort // ""' $(ENV_FILE) 2>/dev/null ))
override USERNAME ?= $(shell ( yq e '.username // ""' $(ENV_FILE) 2>/dev/null ))


.PHONY: read-config
read-config:
	@if [ ! -f "$(CONFIG_FILE)" ] || [ ! -s "$(CONFIG_FILE)" ]; then \
			echo "Error: CONFIG_FILE does not exist or is empty"; exit 1; \
	fi
	@if [ ! -f "$(ENV_FILE)" ] || [ ! -s "$(ENV_FILE)" ]; then \
		echo "Error: ENV_FILE does not exist or is empty"; exit 1; \
	fi
	@echo "static file vars:"
	@echo "target_namespace = $(TARGET_NAMESPACE)"
	@echo "storage_cendor = $(STORAGE_VENDOR)"
	@echo "secret_name = $(SECRET_NAME)"
	@echo "kubeconfig = $(KUBECONFIG)"
	@echo "dynamic file vars:"
	@echo "target_pvc = $(TARGET_PVC)"
	@echo "source_vmdk = $(SOURCE_VMDK)"
	@echo "backend = $(BACKEND)"
	@echo "password = $(PASSWORD)"
	@echo "service_name = $(SERVICE_NAME)"
	@echo "service_port = $(SERVICE_PORT)"
	@echo "username = $(USERNAME)"

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


