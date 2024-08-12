# force makefile to use bash instead of sh.
SHELL := /usr/bin/env bash

.PHONY: all
all: revad reva test-go lint gen-doc


################################################################################
# Toolchain
################################################################################

TOOLCHAIN		?= $(CURDIR)/toolchain
GOLANGCI_LINT	?= $(TOOLCHAIN)/golangci-lint
CALENS			?= $(TOOLCHAIN)/calens

.PHONY: toolchain
toolchain: $(GOLANGCI_LINT) $(CALENS)

$(GOLANGCI_LINT):
	@mkdir -p $(@D)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | BINDIR=$(@D) sh -s v1.54.2

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
BUILD_FLAGS     = -X github.com/cs3org/reva/cmd/revad.gitCommit=$(GIT_COMMIT) -X github.com/cs3org/reva/cmd/revad.version=$(VERSION) -X github.com/cs3org/reva/cmd/revad.goVersion=$(GO_VERSION) -X github.com/cs3org/reva/cmd/revad.buildDate=$(BUILD_DATE)

.PHONY: revad
revad:
	go build -ldflags "$(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: revad-static
revad-static:
	go build -ldflags "-extldflags=-static $(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: gaia
gaia:
	go install github.com/cs3org/gaia@latest

.PHONY: cernbox-revad
cernbox-revad: gaia
	gaia build --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva=$(shell pwd) -o ./cmd/revad/revad

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

TEST				  = litmus-1 litmus-2 acceptance-1 acceptance-2
export REVAD_IMAGE	  ?= revad-eos
export EOS_FULL_IMAGE ?= eos-full
export PARTS		  ?= 1
export PART			  ?= 1

.PHONY: $(TEST)
$(TEST): docker-eos-full-tests docker-revad-eos
	docker compose -f ./tests/docker/docker-compose.yml up --force-recreate --always-recreate-deps --build --abort-on-container-exit -V --remove-orphans --exit-code-from $@ $@

.PHONY: test-go
test-go:
	go test $$([[ -z "$(COVER_PROFILE)" ]] && echo "" || echo "-coverprofile=$(COVER_PROFILE)") -race $$(go list ./... | grep -v /tests/integration)

.PHONY: test-integration
test-integration: revad
	go test -race ./tests/integration/...

.PHONY: check-changelog
check-changelog: $(CALENS)
ifndef PR
	$(error PR is not defined)
else
	$(CALENS) | sed -n '/^Changelog for reva unreleased (UNRELEASED)/,/^Details/p' | \
		grep -E '^ \* [[:alpha:]]{3} #$(PR): '
endif

.PHONY: lint
lint: $(GOLANGCI_LINT)
	@$(GOLANGCI_LINT) run || (echo "Tip: many lint errors can be automatically fixed with \"make lint-fix\""; exit 1)

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT)
	gofmt -w .
	$(GOLANGCI_LINT) run --fix


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
