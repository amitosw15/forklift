# -----------------------------------------------------------------------------
# Shell and File Settings
# -----------------------------------------------------------------------------
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

CONFIG_FILE ?= config.yaml
ENV_FILE ?= .env

SHELL := /usr/bin/env bash

# -----------------------------------------------------------------------------
# Static Variables from Config
# -----------------------------------------------------------------------------
TARGET_NAMESPACE = $(shell yq e '.target-namespace' $(CONFIG_FILE))
STORAGE_VENDOR   = $(shell yq e '.storage-vendor' $(CONFIG_FILE))
SECRET_NAME      = $(shell yq e '.secret-name' $(CONFIG_FILE))
KUBECONFIG       = $(shell yq e '.kubeconfig' $(CONFIG_FILE))

# -----------------------------------------------------------------------------
# Dynamic Variables from ENV (overridable)
# -----------------------------------------------------------------------------
override TARGET_PVC   ?= $(shell ( yq e '.target-pvc // ""' $(ENV_FILE) 2>/dev/null ))
override SOURCE_VMDK ?= $(shell ( yq e '.source-vmdk // ""' $(ENV_FILE) 2>/dev/null ))
override BACKEND      ?= $(shell ( yq e '.backend // ""' $(ENV_FILE) 2>/dev/null ))
override PASSWORD     ?= $(shell ( yq e '.password // ""' $(ENV_FILE) 2>/dev/null ))
override SERVICE_NAME ?= $(shell ( yq e '.serviceName // ""' $(ENV_FILE) 2>/dev/null ))
override SERVICE_PORT ?= $(shell ( yq e '.servicePort // ""' $(ENV_FILE) 2>/dev/null ))
override USERNAME     ?= $(shell ( yq e '.username // ""' $(ENV_FILE) 2>/dev/null ))

# -----------------------------------------------------------------------------
# Populator Files (for prerequisites)
# -----------------------------------------------------------------------------
OPENSTACK_POPULATOR ?= xcopy-setup/forklift.konveyor.io_openstackvolumepopulators.yaml
OVIRT_POPULATOR     ?= xcopy-setup/forklift.konveyor.io_ovirtvolumepopulators.yaml
VSPHERE_POPULATOR   ?= xcopy-setup/forklift.konveyor.io_vspherexcopyvolumepopulators.yaml

# -----------------------------------------------------------------------------
# Additional Variables for Certificate-Tool CLI (defaults can be overridden)
# -----------------------------------------------------------------------------
# Use TARGET_NAMESPACE as the default cluster name.
CLUSTER_NAME ?= $(TARGET_NAMESPACE)
VM_NAME      ?= certificate-vm
PVC_NAME     ?= $(TARGET_PVC)

# The CLI binary name
BINARY = certificate-tool

# -----------------------------------------------------------------------------
# Targets
# -----------------------------------------------------------------------------
.PHONY: help deploy-prerequisites read-config build create-env create-vm create-pvc create-cr test-xcopy all clean

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
		/^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

deploy-prerequisites: ## Deploy prerequisite resources.
	@kubectl apply -f $(OPENSTACK_POPULATOR)
	@kubectl apply -f $(OVIRT_POPULATOR)
	@kubectl apply -f $(VSPHERE_POPULATOR)

read-config: ## Read and display configuration from files.
	@if [ ! -f "$(CONFIG_FILE)" ] || [ ! -s "$(CONFIG_FILE)" ]; then \
		echo "Error: CONFIG_FILE does not exist or is empty"; exit 1; \
	fi
	@if [ ! -f "$(ENV_FILE)" ] || [ ! -s "$(ENV_FILE)" ]; then \
		echo "Error: ENV_FILE does not exist or is empty"; exit 1; \
	fi
	@echo "Static file vars:"
	@echo "  target_namespace = $(TARGET_NAMESPACE)"
	@echo "  storage_vendor   = $(STORAGE_VENDOR)"
	@echo "  secret_name      = $(SECRET_NAME)"
	@echo "  kubeconfig       = $(KUBECONFIG)"
	@echo "Dynamic file vars:"
	@echo "  target_pvc       = $(TARGET_PVC)"
	@echo "  source_vmdk      = $(SOURCE_VMDK)"
	@echo "  backend          = $(BACKEND)"
	@echo "  password         = $(PASSWORD)"
	@echo "  service_name     = $(SERVICE_NAME)"
	@echo "  service_port     = $(SERVICE_PORT)"
	@echo "  username         = $(USERNAME)"

build: ## Build the certificate-tool CLI binary.
	go build -o $(BINARY) .

create-populator-env: build ## Run certificate-tool create-env subcommand.
	./$(BINARY) create-populator-env \
				--kubeconfig $(KUBECONFIG_PATH) \
				--test-namespace $(TEST_NAMESPACE) \
				--test-image-label $(TEST_IMAGE_LABEL) \
				--test-labels $(TEST_LABELS) \
				--test-populator-image $(TEST_POPULATOR_IMAGE) \
				--pod-namespace $(POD_NAMESPACE)

create-test-env: build ## Run certificate-tool create-env subcommand.
	./$(BINARY) create-test-env

create-vm: build ## Run certificate-tool create-vm subcommand.
	./$(BINARY) create-vm --vm-name=$(VM_NAME)

test-xcopy: build create-populator-env create-test-env ## Run certificate-tool create-pvc subcommand.
	./$(BINARY) test-xcopy --pvc-name=$(PVC_NAME)



all: create-env create-vm create-pvc create-cr test-xcopy ## Run all steps in sequence.

clean: ## Clean built artifacts.
	rm -f $(BINARY)
