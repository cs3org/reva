# force makefile to use bash instead of sh.
SHELL := /usr/bin/env bash

.PHONY: all
all: revad reva test-go lint gen-doc


################################################################################
# Toolchain
################################################################################

TOOLCHAIN		?= $(CURDIR)/toolchain
CALENS			?= $(TOOLCHAIN)/calens

CALENS_DIR := $(shell mktemp -d)
$(CALENS):
	@mkdir -p $(@D)
	CALENS_DIR=`mktemp -d`
	git clone --depth 1 --branch v0.2.0 -c advice.detachedHead=false https://github.com/restic/calens.git $(CALENS_DIR)
	cd $(CALENS_DIR) && GOBIN=$(@D) go install
	rm -rf $(CALENS_DIR)


################################################################################
# Build
################################################################################

GIT_COMMIT	?= `git rev-parse --short HEAD`
VERSION		?= `git describe --always`
GO_VERSION	?= `go version | awk '{print $$3}'`
BUILD_DATE	= `date +%FT%T%z`
BUILD_FLAGS     = -X github.com/cs3org/reva/v3/cmd/revad.gitCommit=$(GIT_COMMIT) -X github.com/cs3org/reva/v3/cmd/revad.version=$(VERSION) -X github.com/cs3org/reva/v3/cmd/revad.goVersion=$(GO_VERSION) -X github.com/cs3org/reva/v3/cmd/revad.buildDate=$(BUILD_DATE)

.PHONY: revad
revad:
	go build -ldflags "$(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: revad-static
revad-static:
	go build -tags "sqlite_omit_load_extension" -ldflags "-extldflags=-static $(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: revad-static-musl
revad-static-musl:
	@command -v musl-gcc >/dev/null 2>&1 || { echo "Error: musl-gcc not found. Install with: sudo apt install musl-tools"; exit 1; }
	CGO_ENABLED=1 CC=musl-gcc go build -tags "sqlite_omit_load_extension" -ldflags "-extldflags '-static' $(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: gaia
gaia:
	go install github.com/cs3org/gaia@latest

.PHONY: cernbox-revad
cernbox-revad: gaia
	gaia build --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad

.PHONY: cernbox-revad-local
cernbox-revad-local: gaia
	gaia build --with github.com/cernbox/reva-plugins=$(shell pwd)/../reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad


.PHONY: cernbox-revad-ceph
cernbox-revad-ceph: cernbox-revad
	gaia build --tags ceph --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad-ceph

.PHONY: revad-ceph
revad-ceph:
	go build -ldflags "$(BUILD_FLAGS)" -tags ceph -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: reva
reva:
	go build -ldflags "-extldflags=-static $(BUILD_FLAGS)" -o ./cmd/reva/reva ./cmd/reva

.PHONY: docker-reva
docker-reva:
	docker build -f docker/Dockerfile.reva -t reva --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-revad
docker-revad:
	docker build -f docker/Dockerfile.revad -t revad --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-revad-ceph
docker-revad-ceph:
	docker build -f docker/Dockerfile.revad-ceph -t revad-ceph --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-revad-eos
docker-revad-eos:
	docker build -f docker/Dockerfile.revad-eos -t revad-eos --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-eos-full-tests
docker-eos-full-tests:
	docker build -f tests/docker/eos-storage/Dockerfile -t eos-full tests/docker/eos-storage

################################################################################
# Test
################################################################################

TEST				  = litmus-1 litmus-2
export REVAD_IMAGE	  ?= revad-eos
export EOS_FULL_IMAGE ?= eos-full
export PARTS		  ?= 1
export PART			  ?= 1

.PHONY: $(TEST)
$(TEST): docker-eos-full-tests docker-revad-eos
	docker compose -f ./tests/docker/docker-compose.yml up --force-recreate --always-recreate-deps --build --abort-on-container-exit -V --remove-orphans --exit-code-from $@ $@

.PHONY: test-go
test-go:
	go test -v $$([[ -z "$(COVER_PROFILE)" ]] && echo "" || echo "-coverprofile=$(COVER_PROFILE)") -race $$(go list ./... | grep -v /tests/integration)

.PHONY: test-integration
test-integration: revad
	go test -race ./tests/integration/...

.PHONY: test-reva-cli
test-reva-cli:
	@if ! command -v judo >/dev/null 2>&1; then \
		echo "Error: judo not found. Install with: npm install -g @intuit/judo"; \
		exit 1; \
	fi
	judo tests/integration/reva-cli/file-operations.yaml
	judo tests/integration/reva-cli/versions-recycle.yaml
	judo tests/integration/reva-cli/grants.yaml

.PHONY: check-changelog
check-changelog: $(CALENS)
ifndef PR
	$(error PR is not defined)
else
	$(CALENS) | sed -n '/^Changelog for reva unreleased (UNRELEASED)/,/^Details/p' | \
		grep -E '^ \* [[:alpha:]]{3} #$(PR): '
endif

.PHONY: lint
lint:
	go vet ./...

################################################################################
# Release
################################################################################

.PHONY: gen-doc
gen-doc:
	go run tools/generate-documentation/main.go

dist: gen-doc
	go run tools/create-artifacts/main.go -version $(VERSION) -commit $(GIT_COMMIT) -goversion $(GO_VERSION)


################################################################################
# Clean
################################################################################

.PHONY: toolchain-clean
toolchain-clean:
	rm -rf $(TOOLCHAIN)

.PHONY: docker-clean
docker-clean:
	docker compose -f ./tests/docker/docker-compose.yml down --rmi local -v --remove-orphans

.PHONY: clean
clean: toolchain-clean docker-clean
	rm -rf dist
	rm -rf tmp
