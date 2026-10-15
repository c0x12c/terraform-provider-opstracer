VERSION ?= 0.1.0
OS_ARCH := $(shell go env GOOS)_$(shell go env GOARCH)
# Terraform's implied local mirror; `terraform init` finds the provider here without a registry.
PLUGIN_DIR := $(HOME)/.terraform.d/plugins/registry.terraform.io/c0x12c/opstracer/$(VERSION)/$(OS_ARCH)

.PHONY: build test install-local

build:
	go build -ldflags "-X main.version=$(VERSION)" -o terraform-provider-opstracer .

test:
	go test ./...

install-local:
	mkdir -p $(PLUGIN_DIR)
	go build -ldflags "-X main.version=$(VERSION)" -o $(PLUGIN_DIR)/terraform-provider-opstracer_v$(VERSION) .
	@echo "Installed to $(PLUGIN_DIR)"
