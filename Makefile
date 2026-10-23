# force makefile to use bash instead of sh.
SHELL := /usr/bin/env bash

.PHONY: all
all: revad reva test-go lint gen-doc ## Build revad and reva, run the unit tests and the linter, generate the docs

# help lists the targets that have a "## description", grouped under the
# titles of the section banners below.
.PHONY: help
help: ## Show this help
	@if [ -t 1 ] && [ -z "$$NO_COLOR" ]; then h=$$'\e[1;36m' n=$$'\e[1;32m' r=$$'\e[0m'; fi; \
	awk -v h="$$h" -v n="$$n" -v r="$$r" -v test="$(TEST)" ' \
		BEGIN { printf "%sUsage:%s\n  make <target>\n\n%sTargets:%s\n", h, r, h, r } \
		/^####/ { banner = 1; next } \
		banner && /^# / { section = substr($$0, 3) } \
		{ banner = 0 } \
		match($$0, /^[a-zA-Z0-9_$$().-]+:/) && index($$0, " ## ") { \
			name = substr($$0, 1, RLENGTH - 1); sub(/\$$\(TEST\)/, test, name); \
			if (section != "") { printf "\n%s%s:%s\n", h, section, r; section = "" } \
			printf "  %s%-26s%s %s\n", n, name, r, substr($$0, index($$0, " ## ") + 4) \
		}' $(MAKEFILE_LIST)


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
revad: ## Build revad
	go build -ldflags "$(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: revad-static
revad-static: ## Build a static revad
	go build -tags "sqlite_omit_load_extension" -ldflags "-extldflags=-static $(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: revad-static-musl
revad-static-musl: ## Build a static revad with musl
	@command -v musl-gcc >/dev/null 2>&1 || { echo "Error: musl-gcc not found. Install with: sudo apt install musl-tools"; exit 1; }
	CGO_ENABLED=1 CC=musl-gcc go build -tags "sqlite_omit_load_extension" -ldflags "-extldflags '-static' $(BUILD_FLAGS)" -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: gaia
gaia: ## Install the gaia build tool
	go install github.com/cs3org/gaia@latest

.PHONY: cernbox-revad
cernbox-revad: gaia ## Build revad with the CERNBox plugins
	gaia build --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad

.PHONY: cernbox-revad-local
cernbox-revad-local: gaia ## Build revad with the CERNBox plugins from ../reva-plugins
	gaia build --with github.com/cernbox/reva-plugins=$(shell pwd)/../reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad


.PHONY: cernbox-revad-ceph
cernbox-revad-ceph: cernbox-revad ## Build revad with the CERNBox plugins and Ceph
	gaia build --tags ceph --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad-ceph

.PHONY: cernbox-revad-ceph-local
cernbox-revad-ceph-local: gaia ## Build revad with the CERNBox plugins from ../reva-plugins and Ceph
	gaia build --tags ceph --with github.com/cernbox/reva-plugins=$(shell pwd)/../reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad-ceph

.PHONY: revad-ceph
revad-ceph: ## Build revad with Ceph
	go build -ldflags "$(BUILD_FLAGS)" -tags ceph -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: reva
reva: ## Build the reva CLI
	go build -ldflags "-extldflags=-static -X main.gitCommit=$(GIT_COMMIT) -X main.version=$(VERSION) -X main.goVersion=$(GO_VERSION) -X main.buildDate=$(BUILD_DATE)" -o ./cmd/reva/reva ./cmd/reva

# Regenerate the reva-owned Admin API (pkg/admin/adminpb) and control channel
# (pkg/control/controlpb) stubs from their protos. Needs protoc plus protoc-gen-go and
# protoc-gen-go-grpc on PATH.
.PHONY: protobuf
protobuf: ## Regenerate the admin and control channel gRPC stubs
	protoc --proto_path=pkg/admin/proto \
		--go_out=. --go_opt=module=github.com/cs3org/reva/v3 \
		--go-grpc_out=. --go-grpc_opt=module=github.com/cs3org/reva/v3 \
		pkg/admin/proto/admin.proto
	protoc --proto_path=pkg/control/proto \
		--go_out=. --go_opt=module=github.com/cs3org/reva/v3 \
		--go-grpc_out=. --go-grpc_opt=module=github.com/cs3org/reva/v3 \
		pkg/control/proto/control.proto

.PHONY: docker-reva
docker-reva: ## Build the reva docker image
	docker build -f docker/Dockerfile.reva -t reva --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-revad
docker-revad: ## Build the revad docker image
	docker build -f docker/Dockerfile.revad -t revad --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-revad-ceph
docker-revad-ceph: ## Build the revad docker image with Ceph
	docker build -f docker/Dockerfile.revad-ceph -t revad-ceph --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-revad-eos
docker-revad-eos: ## Build the revad docker image for EOS
	docker build -f docker/Dockerfile.revad-eos -t revad-eos --build-arg VERSION=$(VERSION) --build-arg GIT_COMMIT=$(GIT_COMMIT) .

.PHONY: docker-eos-full-tests
docker-eos-full-tests: ## Build the EOS docker image for the litmus tests
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
$(TEST): docker-eos-full-tests docker-revad-eos ## Run the litmus WebDAV tests in docker
	docker compose -f ./tests/docker/docker-compose.yml up --force-recreate --always-recreate-deps --build --abort-on-container-exit -V --remove-orphans --exit-code-from $@ $@

.PHONY: test-go
test-go: ## Run the unit tests
	go test $$([[ -z "$(COVER_PROFILE)" ]] && echo "" || echo "-coverprofile=$(COVER_PROFILE)") -race $$(go list ./... | grep -v /tests/integration)

.PHONY: test-integration
test-integration: revad ## Run the integration tests (needs Redis and MySQL)
	go test -race ./tests/integration/...

.PHONY: test-reva-cli
test-reva-cli: ## Run the reva CLI tests with judo (needs a running revad)
	@if ! command -v judo >/dev/null 2>&1; then \
		echo "Error: judo not found. Install with: npm install -g @intuit/judo"; \
		exit 1; \
	fi
	judo tests/integration/reva-cli/file-operations.yaml
	judo tests/integration/reva-cli/versions-recycle.yaml
	judo tests/integration/reva-cli/grants.yaml

.PHONY: check-changelog
check-changelog: $(CALENS) ## Check the changelog entry of PR=<number>
ifndef PR
	$(error PR is not defined)
else
	$(CALENS) | sed -n '/^Changelog for reva unreleased (UNRELEASED)/,/^Details/p' | \
		grep -E '^ \* [[:alpha:]]{3} #$(PR): '
endif

.PHONY: lint
lint: ## Run go vet
	go vet ./...

################################################################################
# Release
################################################################################

.PHONY: gen-doc
gen-doc: ## Generate the config reference of the docs
	go run tools/generate-documentation/main.go

dist: gen-doc ## Build the release artifacts
	go run tools/create-artifacts/main.go -version $(VERSION) -commit $(GIT_COMMIT) -goversion $(GO_VERSION)

# use the following as `NEWVER=3.x.y make release`
release: gen-doc ## Prepare a release (NEWVER=3.x.y)
	go run tools/prepare-release/main.go -version $(NEWVER) -commit -tag


################################################################################
# Clean
################################################################################

.PHONY: toolchain-clean
toolchain-clean: ## Remove the downloaded tools
	rm -rf $(TOOLCHAIN)

.PHONY: docker-clean
docker-clean: ## Remove the docker test containers and images
	docker compose -f ./tests/docker/docker-compose.yml down --rmi local -v --remove-orphans

.PHONY: clean
clean: toolchain-clean docker-clean ## Remove the build and test outputs
	rm -rf dist
	rm -rf tmp
