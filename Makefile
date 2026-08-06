# skm — adopts alswl/makefile-go (https://github.com/alswl/makefile-go)
#
# The core modules live under hack/makefile-go/ (DO NOT EDIT) and are included
# below. This file only configures them for this project and overrides the
# build / test / version targets where this project's conventions differ.
#
#   make              - default to fmt test build
#   make build        - build ./bin/skm (version injected via pkg/version)
#   make test         - run unit tests
#   make install      - install bin/skm to PATH
#   make lint         - gofmt + golangci-lint
#   make help         - list all targets

SHELL := /bin/bash

PROJECT := skm
# Single binary target.
TARGETS := skm

# Container registries (unused: skm builds no images).
REGISTRIES ?= ""
IMAGE_PREFIX ?= $(strip )
IMAGE_SUFFIX ?= $(strip )

# This repo's root import path.
ROOT := github.com/alswl/skm/skm
# Project main package location.
CMD_DIR := ./cmd
# Project output directory.
OUTPUT_DIR := ./bin
# Build directory.
BUILD_DIR := ./build

PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))

# Git commit sha (makefile-go convention: -dirty suffix on uncommitted trees).
COMMIT := $(strip $(shell git rev-parse --short HEAD 2>/dev/null))
COMMIT := $(COMMIT)$(shell [[ -z $$(git status -s) ]] || echo '-dirty')
COMMIT := $(if $(COMMIT),$(COMMIT),none)

# Version: git describe fallback (no VERSION file in this project).
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
BUILD_VERSION = $(COMMIT)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOOS = $(shell go env GOOS)
GOARCH = $(shell go env GOARCH)
GOPATH = $(shell go env GOPATH)
CGO_ENABLED = $(shell go env CGO_ENABLED)

UT_COVER_PACKAGES := $(shell go list ./pkg/... | grep -Ev 'pkg/version')
COVERAGE_PACKAGES := $(shell go list ./pkg/... | awk '{printf "%s%s", sep, $$0; sep=","} END{print ""}')
COVERAGE_PROFILING_DIR := $(PROJECT_DIR)/.cover

.PHONY: all
all: fmt test build

include hack/makefile-go/_git.mk
include hack/makefile-go/build.mk
include hack/makefile-go/install.mk
include hack/makefile-go/test.mk
include hack/makefile-go/general.mk
include hack/makefile-go/version.mk

##@ Self defined
.PHONY: run
run: ## Run the TUI locally
	go run $(CMD_DIR)/skm

.PHONY: tidy
tidy: ## Tidy go modules
	go mod tidy

##@ Override (project conventions)

.PHONY: build
build: ## Build the skm binary into bin/ with version metadata
	@mkdir -p $(OUTPUT_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o $(OUTPUT_DIR)/skm \
		-ldflags "-s -w \
		-X $(ROOT)/pkg/version.Version=$(VERSION) \
		-X $(ROOT)/pkg/version.Commit=$(COMMIT) \
		-X $(ROOT)/pkg/version.Date=$(DATE)" \
		$(CMD_DIR)/skm

.PHONY: test
test: ## Run unit tests (plain go test; makefile-go's -race/-N skews perf baselines)
	go test ./...

.PHONY: version
version: ## Print the computed version
	@echo "$(VERSION) ($(COMMIT))"
