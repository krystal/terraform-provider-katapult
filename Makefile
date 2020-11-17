HOSTNAME=katapult.io
NAMESPACE=katapult
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

# Global environment variables for all targets
SHELL ?= /bin/bash
SHELL := env \
	GO111MODULE=on \
	GOBIN=$(CURDIR)/$(TOOLDIR) \
	CGO_ENABLED=1 \
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

TOOLS += $(TOOLDIR)/gobin
gobin: $(TOOLDIR)/gobin
$(TOOLDIR)/gobin:
	GO111MODULE=off go get -u github.com/myitcv/gobin

# external tool
define tool # 1: binary-name, 2: go-import-path
TOOLS += $(TOOLDIR)/$(1)

.PHONY: $(1)
$(1): $(TOOLDIR)/$(1)

$(TOOLDIR)/$(1): $(TOOLDIR)/gobin Makefile
	gobin $(V) "$(2)"
endef

$(eval $(call tool,gofumports,mvdan.cc/gofumpt/gofumports))
$(eval $(call tool,golangci-lint,github.com/golangci/golangci-lint/cmd/golangci-lint@v1.31))

.PHONY: tools
tools: $(TOOLS)

#
# Build
#

BINARY=terraform-provider-$(NAME)
LDFLAGS := -w -s

VERSION ?= $(shell git describe --tags 2>/dev/null)
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null)
DATE ?= $(shell date +%s)

ifeq ($(trim $(VERSION)),)
	VERSION = 0.0.1
endif

.PHONY: build
build: $(BINARY)

$(BINARY): $(SOURCES)
	CGO_ENABLED=0 go build $(V) -a -o "$@" -ldflags "$(LDFLAGS) \
		-X main.Version=$(VERSION) \
		-X main.Commit=$(GIT_SHA) \
		-X main.Date=$(DATE)"

TF_PLUGINS ?= $(HOME)/.terraform.d/plugins
INSTALL_DIR = $(TF_PLUGINS)/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(VERSION)

.PHONY: install
install: build
	$(eval OS_ARCH := $(shell go env GOOS)_$(shell go env GOARCH))
	mkdir -p "$(INSTALL_DIR)/$(OS_ARCH)"
	cp "$(BINARY)" "$(INSTALL_DIR)/$(OS_ARCH)/"

#
# Development
#

TEST ?= $$(go list ./... | grep -v 'vendor')
SWEEP_DIR ?= ./internal/provider

.PHONY: clean
clean:
	rm -rf $(BINARY) $(TOOLS)
	rm -f ./coverage.out ./go.mod.tidy-check ./go.sum.tidy-check

.PHONY: test
test:
	go test $(V) -count=1 -race $(TESTARGS) $(TEST)

.PHONY: testacc
testacc:
	TF_ACC=1 go test $(V) $(TESTARGS) $(TEST) -timeout=120m -parallel=10

.PHONY: test-update
test-update-golden:
	go test $(V) -update-golden -count=1 -race $(TESTARGS) $(TEST)

.PHONY: test-deps
test-deps:
	go test all

.PHONY: lint
lint: golangci-lint
	GOGC=off golangci-lint $(V) run

.PHONY: format
format: gofumports
	gofumports -w .

sweep:
	$(info WARNING: This will destroy infrastructure. Use only on \
		development accounts.)
	go test $(SWEEP_DIR) -v -sweep=all $(SWEEPARGS) -timeout 60m

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
	TF_ACC=1 VCR=replay go test $(V) -timeout=120m -parallel=10 \
			-covermode=count -coverprofile=coverage.out \
			$(TESTARGS) $(TEST)

#
# Dependencies
#

.PHONY: deps
deps:
	$(info Downloading dependencies)
	go mod download

.PHONY: tidy
tidy:
	go mod tidy $(V)

.PHONY: verify
verify:
	go mod verify

.SILENT: check-tidy
.PHONY: check-tidy
check-tidy:
	cp go.mod go.mod.tidy-check
	cp go.sum go.sum.tidy-check
	go mod tidy
	-diff go.mod go.mod.tidy-check
	-diff go.sum go.sum.tidy-check
	-rm -f go.mod go.sum
	-mv go.mod.tidy-check go.mod
	-mv go.sum.tidy-check go.sum
