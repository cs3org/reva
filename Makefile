# force makefile to use bash instead of sh.
SHELL := /usr/bin/env bash

.PHONY: all
all: revad reva test-go lint gen-doc ## Build revad and reva, run the unit tests and the linter, generate the docs

HELP_COLORS = if [ -t 1 ] && [ -z "$$NO_COLOR" ]; then h=$$'\e[1;36m' n=$$'\e[1;32m' v=$$'\e[33m' r=$$'\e[0m'; fi

# help lists the targets that have a "## description", grouped under the
# titles of the section banners below.
.PHONY: help
help: ## Show this help
	@$(HELP_COLORS); \
	awk -v h="$$h" -v n="$$n" -v r="$$r" -v test="$(TEST)" ' \
		BEGIN { printf "%sUsage:%s\n  make <target>\n\n%sTargets:%s\n", h, r, h, r } \
		/^####/ { banner = 1; next } \
		banner && /^# / { section = substr($$0, 3) } \
		{ banner = 0 } \
		match($$0, /^[a-zA-Z0-9_$$().-]+:/) && index($$0, " ## ") { \
			name = substr($$0, 1, RLENGTH - 1); sub(/\$$\(TEST\)/, test, name); \
			if (section != "") { printf "\n%s%s:%s\n", h, section, r; section = "" } \
			printf "  %s%-26s%s %s\n", n, name, r, substr($$0, index($$0, " ## ") + 4) \
		} \
		END { printf "\nUse \"make help-<target>\" for more information about a target.\n" }' $(MAKEFILE_LIST)

# help-<target> prints the description of a target, the comment block
# right above it and its prerequisites. Write the details and the variables
# of a target in that comment block.
help-%:
	@$(HELP_COLORS); \
	awk -v h="$$h" -v n="$$n" -v v="$$v" -v r="$$r" -v test="$(TEST)" -v want="$*" ' \
		/^#/ && !/^####/ { line = $$0; sub(/^# ?/, "", line); block[nb++] = line; next } \
		/^\.PHONY:/ { next } \
		match($$0, /^[a-zA-Z0-9_$$().-]+:/) && index($$0, " ## ") { \
			name = substr($$0, 1, RLENGTH - 1); sub(/\$$\(TEST\)/, test, name); \
			if (index(" " name " ", " " want " ")) { \
				d = index($$0, " ## "); deps = substr($$0, RLENGTH + 1, d - RLENGTH - 1); gsub(/^ +| +$$/, "", deps); \
				printf "%s%s%s: %s\n", n, want, r, substr($$0, d + 4); \
				if (nb) printf "\n"; \
				for (i = 0; i < nb; i++) { \
					l = block[i]; \
					if (l ~ /^[A-Z][A-Za-z ()]*:$$/) l = h l r; \
					else if (match(l, /^  [A-Z_]+/)) l = "  " v substr(l, 3, RLENGTH - 2) r substr(l, RLENGTH + 1); \
					print l \
				} \
				if (deps != "") printf "\n%sDepends on:%s %s\n", h, r, deps; \
				found = 1; exit \
			} \
		} \
		{ nb = 0 } \
		END { if (!found) { print "No help for \"" want "\". Run \"make help\" to list the targets." > "/dev/stderr"; exit 1 } }' $(MAKEFILE_LIST)


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
BUILD_FLAGS     = -X github.com/cs3org/reva/v3/pkg/rversion.GitCommit=$(GIT_COMMIT) -X github.com/cs3org/reva/v3/pkg/rversion.Version=$(VERSION) -X github.com/cs3org/reva/v3/pkg/rversion.GoVersion=$(GO_VERSION) -X github.com/cs3org/reva/v3/pkg/rversion.BuildDate=$(BUILD_DATE)

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

# Build cmd/revad/revad with gaia, with the plugins of
# github.com/cernbox/reva-plugins and this checkout of reva.
.PHONY: cernbox-revad
cernbox-revad: gaia ## Build revad with the CERNBox plugins
	gaia build --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad

# Like cernbox-revad, but take the plugins from your checkout in
# ../reva-plugins instead of the published module.
.PHONY: cernbox-revad-local
cernbox-revad-local: gaia ## Build revad with the CERNBox plugins from ../reva-plugins
	gaia build --with github.com/cernbox/reva-plugins=$(shell pwd)/../reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad


# Like cernbox-revad, with the ceph build tag. It writes
# cmd/revad/revad-ceph (and cmd/revad/revad, from cernbox-revad).
.PHONY: cernbox-revad-ceph
cernbox-revad-ceph: cernbox-revad ## Build revad with the CERNBox plugins and Ceph
	gaia build --tags ceph --with github.com/cernbox/reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad-ceph

# Like cernbox-revad-local, with the ceph build tag. It writes
# cmd/revad/revad-ceph.
.PHONY: cernbox-revad-ceph-local
cernbox-revad-ceph-local: gaia ## Build revad with the CERNBox plugins from ../reva-plugins and Ceph
	gaia build --tags ceph --with github.com/cernbox/reva-plugins=$(shell pwd)/../reva-plugins --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/revad/revad-ceph

.PHONY: revad-ceph
revad-ceph: ## Build revad with Ceph
	go build -ldflags "$(BUILD_FLAGS)" -tags ceph -o ./cmd/revad/revad ./cmd/revad/main

.PHONY: reva
reva: ## Build the reva CLI
	go build -ldflags "-extldflags=-static $(BUILD_FLAGS)" -o ./cmd/reva/reva ./cmd/reva

# The CERNBox build of the CLI: reva with the CLI plugins of
# github.com/cernbox/reva-plugins/cli and the CERNBox brand. It needs gaia
# with --binary, --brand and a subpackage in --with (T35), and the brand
# file of reva-plugins (T42). Until then these targets fail.
CERNBOX_BRAND ?= $(shell pwd)/../reva-plugins/cli/cernbox.toml

.PHONY: cernbox
cernbox: gaia ## Build the cernbox CLI (reva with the CERNBox CLI plugins)
	gaia build --binary reva --brand $(CERNBOX_BRAND) --with github.com/cernbox/reva-plugins/cli --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/reva/cernbox

.PHONY: cernbox-local
cernbox-local: gaia ## Build the cernbox CLI with the CLI plugins from ../reva-plugins
	gaia build --binary reva --brand $(CERNBOX_BRAND) --with github.com/cernbox/reva-plugins/cli=$(shell pwd)/../reva-plugins/cli --with github.com/cs3org/reva/v3=$(shell pwd) --debug -o ./cmd/reva/cernbox

# Regenerate the reva-owned Admin API (pkg/admin/adminpb) and control channel
# (pkg/control/controlpb) stubs from their protos. Needs protoc plus protoc-gen-go and
# protoc-gen-go-grpc on PATH, and a cs3apis checkout for the CS3 imports.
CS3APIS_DIR ?= ../cs3apis
# The CS3 protos get their Go package from buf, not from go_package, so protoc
# needs one M option for each of them.
CS3APIS_GO_OPTS = $(foreach f,$(shell cd $(CS3APIS_DIR) && find cs3 -name '*.proto'),M$(f)=github.com/cs3org/go-cs3apis/$(patsubst %/,%,$(dir $(f))))

.PHONY: protobuf
protobuf: ## Regenerate the admin and control channel gRPC stubs (CS3APIS_DIR=<cs3apis checkout>)
	@test -d $(CS3APIS_DIR)/cs3 || { echo "protobuf: set CS3APIS_DIR to a checkout of github.com/cs3org/cs3apis"; exit 1; }
	protoc --proto_path=pkg/admin/proto --proto_path=$(CS3APIS_DIR) \
		--go_out=. --go_opt=module=github.com/cs3org/reva/v3 $(addprefix --go_opt=,$(CS3APIS_GO_OPTS)) \
		--go-grpc_out=. --go-grpc_opt=module=github.com/cs3org/reva/v3 $(addprefix --go-grpc_opt=,$(CS3APIS_GO_OPTS)) \
		pkg/admin/proto/*.proto
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

# Build the EOS and revad images, then run the litmus WebDAV tests with
# docker compose (tests/docker/docker-compose.yml).
#
# Variables:
#   EOS_FULL_IMAGE  the EOS image to test against (default eos-full)
.PHONY: $(TEST)
$(TEST): docker-eos-full-tests docker-revad-eos ## Run the litmus WebDAV tests in docker
	docker compose -f ./tests/docker/docker-compose.yml up --force-recreate --always-recreate-deps --build --abort-on-container-exit -V --remove-orphans --exit-code-from $@ $@

# Run the unit tests with the race detector. The tests in tests/integration
# are left out: use test-integration for them.
#
# Variables:
#   COVER_PROFILE  write a coverage profile to this file
.PHONY: test-go
test-go: ## Run the unit tests
	go test $$([[ -z "$(COVER_PROFILE)" ]] && echo "" || echo "-coverprofile=$(COVER_PROFILE)") -race $$(go list ./... | grep -v /tests/integration)

# Build revad, then run the gRPC integration tests in tests/integration.
# They need a Redis and a MySQL server.
#
# Variables (environment):
#   REDIS_ADDRESS  address of Redis, for example localhost:6379
#   SQL_USERNAME   MySQL user
#   SQL_PASSWORD   MySQL password
#   SQL_ADDRESS    address of MySQL, for example localhost:3306
#   SQL_DBNAME     MySQL database
.PHONY: test-integration
test-integration: revad ## Run the integration tests (needs Redis and MySQL)
	go test -race ./tests/integration/...

# Run the reva CLI tests in tests/integration/reva-cli with judo
# (npm install -g @intuit/judo). REVA_TOKEN_FILE keeps the login token of the
# tests out of the user's config dir. Build reva first and start revad with
# tests/integration/reva-cli/config/revad-localfs.toml: see
# .github/workflows/test-reva-cli.yml for the setup.
.PHONY: test-reva-cli
test-reva-cli: ## Run the reva CLI tests with judo (needs a running revad)
	@if ! command -v judo >/dev/null 2>&1; then \
		echo "Error: judo not found. Install with: npm install -g @intuit/judo"; \
		exit 1; \
	fi
	REVA_TOKEN_FILE=token judo tests/integration/reva-cli/file-operations.yaml
	REVA_TOKEN_FILE=token judo tests/integration/reva-cli/versions-recycle.yaml
	REVA_TOKEN_FILE=token judo tests/integration/reva-cli/grants.yaml

# Check that the unreleased changelog has an entry that links the pull
# request. CI runs it on every pull request.
#
# Variables:
#   PR  number of the pull request (required)
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

# Run make all, move changelog/unreleased into changelog/<version>_<date>,
# regenerate CHANGELOG.md and write VERSION and RELEASE_DATE. Then commit
# and tag the release.
#
# Variables:
#   NEWVER  the version to release, for example 3.14.1 (required)
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
