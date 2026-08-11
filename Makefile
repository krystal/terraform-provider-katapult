HOSTNAME=registry.terraform.io
NAMESPACE=krystal
NAME=katapult

SOURCES := $(shell find . -name "*.go" -or -name "go.mod" -or -name "go.sum" \
	-or -name "Makefile")

#
# Environment
#

# Verbose output
ifdef VERBOSE
V = -v
endif

BINDIR := bin
TOOLDIR := $(BINDIR)/tools
GO_ROOT := $(shell go env GOROOT)
GO_BINARY := $(GO_ROOT)/bin/go

# Global environment variables for all targets
SHELL ?= /bin/bash
SHELL := env \
	GO111MODULE=on \
	GOBIN=$(CURDIR)/$(BINDIR) \
	CGO_ENABLED=0 \
	PATH='$(CURDIR)/$(BINDIR):$(CURDIR)/$(TOOLDIR):$(PATH)' \
	$(SHELL)

#
# Defaults
#

# Default target
.DEFAULT_GOAL := build

#
# Tools
#

# external tool
define tool # 1: binary-name, 2: go-import-path
TOOLS += $(TOOLDIR)/$(1)

$(TOOLDIR)/$(1): Makefile
	GOROOT="$(GO_ROOT)" GOBIN="$(CURDIR)/$(TOOLDIR)" \
		"$(GO_BINARY)" install "$(2)"
endef

$(eval $(call tool,golangci-lint,github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2))
$(eval $(call tool,gomod,github.com/Helcaraxan/gomod@v0.7.1))

.PHONY: tools
tools: $(TOOLS)

#
# Build
#

BINARY=bin/terraform-provider-$(NAME)
LDFLAGS := -w -s

VERSION ?= $(shell git describe --tags --exact 2>/dev/null)
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null)

# Major and minor dev version should match that of the latest version, but patch
# should always be 999.
DEV_VERSION = 0.0.999

ifeq ($(VERSION),)
	VERSION = $(DEV_VERSION)
endif

.PHONY: build
build: $(BINARY)

$(BINARY): $(SOURCES)
	go build $(V) -a -o "$@" -ldflags "$(LDFLAGS) \
		-X main.version=$(VERSION) \
		-X main.commit=$(GIT_SHA)"

TF_PLUGINS ?= $(HOME)/.terraform.d/plugins_local
INSTALL_DIR = $(TF_PLUGINS)/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(DEV_VERSION)

.PHONY: install
install:
	@echo "Please configure your $(HOME)/.terraformrc file something like this:"
	@echo ""
	@echo "    provider_installation {"
	@echo "      filesystem_mirror {"
	@echo "        path    = \"$(HOME)/.terraform.d/plugins_local/\""
	@echo "        include = [\"registry.terraform.io/krystal/katapult\"]"
	@echo "      }"
	@echo "      direct {"
	@echo "        exclude = [\"registry.terraform.io/krystal/katapult\"]"
	@echo "      }"
	@echo "    }"
	@echo ""
	@echo "You MUST comment out the 'version' constraint in the required_providers"
	@echo "block in any Terraform installation you test this in."
	@echo ""
	@echo "You MUST delete existing cached plugins from any .terraform directories"
	@echo "in Terraform installations you want to test against so that it will"
	@echo "perform a lookup on the local mirror"
	@echo ""
	$(eval OS_ARCH := $(shell go env GOOS)_$(shell go env GOARCH))
	go build $(V) -o "$(INSTALL_DIR)/$(OS_ARCH)/$(notdir $(BINARY))" \
		-ldflags "$(LDFLAGS) \
		-X main.version=$(DEV_VERSION) \
		-X main.commit=$(GIT_SHA)"

#
# Development
#

TEST ?= $$(go list ./... | grep -v 'vendor')
SWEEP_DIR ?= ./internal/provider
SWEEP_V6_DIR ?= ./internal/v6provider

.PHONY: clean
clean:
	rm -rf $(BINARY) $(TOOLS)
	rm -f ./coverage.out ./go.mod.tidy-check ./go.sum.tidy-check

.PHONY: clean-cassettes
clean-cassettes:
	rm -f $(shell find * -path '*/testdata/*' -name '*.cassette.*')

.PHONY: test
test:
	CGO_ENABLED=1 go test $(V) -count=1 -race $(TESTARGS) $(TEST)

.PHONY: testacc
testacc:
	TF_ACC=1 go test $(V) -count=1 $(TESTARGS) $(TEST) -timeout=120m

.PHONY: test-deps
test-deps:
	go test all

.PHONY: lint
lint: $(TOOLDIR)/golangci-lint
	golangci-lint $(V) run

.PHONY: format
format: $(TOOLDIR)/golangci-lint
	golangci-lint $(V) run --fix

sweep:
	$(info WARNING: This will destroy infrastructure. Use only on \
		development accounts.)
	go test $(SWEEP_V6_DIR) -v -sweep=all $(SWEEPARGS) -timeout 60m
	go test $(SWEEP_DIR) -v -sweep=all $(SWEEPARGS) -timeout 60m

.PHONY: shell
shell: docker-dev-build
	$(eval IMAGE := $(shell $(DOCKER_DEV_BUILD_CMD) -q))
	docker run --rm -ti \
		-v "$(CURDIR)/:/terraform-provider-katapult/" \
		-v "katapult-terraform-provider-bins:/terraform-provider-katapult/bin" \
		-v "katapult-terraform-provider-gomod-cache:/go/pkg/mod" \
		"$(IMAGE)" bash

.PHONY: shell-clean
shell-clean:
	docker volume rm katapult-terraform-provider-bins
	docker volume rm katapult-terraform-provider-gomod-cache

DOCKER_DEV_BUILD_CMD = docker build -f Dockerfile.dev .

.PHONY: docker-dev-build
docker-dev-build:
	$(DOCKER_DEV_BUILD_CMD)

#
# Coverage
#

.PHONY: cov
cov: coverage.out

.PHONY: cov-html
cov-html: coverage.out
	go tool cover -html=coverage.out

.PHONY: cov-func
cov-func: coverage.out
	go tool cover -func=coverage.out

coverage.out: $(SOURCES)
	TF_ACC=0 VCR=replay go test $(V) -timeout=120m \
			-covermode=count -coverprofile=coverage.out \
			$(TESTARGS) $(TEST)

#
# Dependencies
#

.PHONY: deps
deps:
	$(info Downloading dependencies)
	go mod download

.PHONY: deps-update
deps-update:
	$(info Downloading dependencies)
	go get -u -t ./...

.PHONY: deps-analyze
deps-analyze: $(TOOLDIR)/gomod
	gomod analyze

.PHONY: tidy
tidy:
	go mod tidy $(V)

.PHONY: verify-deps
verify-deps:
	go mod verify

# Deprecated compatibility alias. Use `mise run verify` for the broad
# pre-handoff validation suite.
.PHONY: verify
verify: verify-deps
	@echo "make verify only verifies downloaded Go modules; use mise run verify for the full suite"

.PHONY: check-tidy
check-tidy:
	go mod tidy -diff
