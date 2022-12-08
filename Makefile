SHELL := /bin/bash
BUILD_DATE=`date +%FT%T%z`
GIT_COMMIT ?= `git rev-parse --short HEAD`
GIT_BRANCH=`git rev-parse --symbolic-full-name --abbrev-ref HEAD`
GIT_DIRTY=`git diff-index --quiet HEAD -- || echo "dirty-"`
VERSION	?= `git describe --always`
GO_VERSION ?= `go version | awk '{print $$3}'`
MINIMUM_GO_VERSION=1.16.2
BUILD_FLAGS="-X main.gitCommit=${GIT_COMMIT} -X main.version=${VERSION} -X main.goVersion=${GO_VERSION} -X main.buildDate=${BUILD_DATE}"
CI_BUILD_FLAGS="-w -extldflags "-static" -X main.gitCommit=${GIT_COMMIT} -X main.version=${VERSION} -X main.goVersion=${GO_VERSION} -X main.buildDate=${BUILD_DATE}"
LITMUS_URL_OLD="http://localhost:20080/remote.php/webdav"
LITMUS_URL_NEW="http://localhost:20080/remote.php/dav/files/4c510ada-c86b-4815-8820-42cdf82c3d51"
LITMUS_USERNAME="einstein"
LITMUS_PASSWORD="relativity"
TESTS="basic http copymove props"

TOOLCHAIN		?= $(CURDIR)/toolchain
GOLANGCI_LINT	?= $(TOOLCHAIN)/golangci-lint
CALENS			?= $(TOOLCHAIN)/calens
GOIMPORTS		?= $(TOOLCHAIN)/goimports

include .bingo/Variables.mk

.PHONY: toolchain
toolchain: $(GOLANGCI_LINT) $(CALENS) $(GOIMPORTS)

.PHONY: toolchain-clean
toolchain-clean:
	rm -rf $(TOOLCHAIN)

$(GOLANGCI_LINT):
	@mkdir -p $(@D)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | BINDIR=$(@D) sh -s v1.50.1

.PHONY: check-changelog
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --timeout 300s

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT)
	gofmt -w .
	$(GOLANGCI_LINT) run --fix

$(CALENS):
	@mkdir -p $(@D)
	git clone --depth 1 --branch v0.2.0 -c advice.detachedHead=false https://github.com/restic/calens.git /tmp/calens
	cd /tmp/calens && GOBIN=$(@D) go install
	rm -rf /tmp/calens

.PHONY: check-changelog
check-changelog: $(CALENS)
ifndef PR
	$(error PR is not defined)
else
	$(CALENS) | sed -n '/^Changelog for reva unreleased (UNRELEASED)/,/^Details/p' | \
		grep -E '^\*   [[:alpha:]]{3} #$(PR): '
endif

$(GOIMPORTS):
	@mkdir -p $(@D)
	GOBIN=$(@D) go install golang.org/x/tools/cmd/goimports@v0.3.0

.PHONY: off
off:
	GOPROXY=off
	echo BUILD_DATE=${BUILD_DATE}
	echo GIT_COMMIT=${GIT_COMMIT}
	echo GIT_DIRTY=${GIT_DIRTY}
	echo VERSION=${VERSION}
	echo GO_VERSION=${GO_VERSION}

.PHONY: imports
imports: off $(GOIMPORTS)
	$(GOIMPORTS) -w tools pkg internal cmd

.PHONY: build
build: build-revad build-reva test-go-version

.PHONY: build-cephfs
build-cephfs: build-revad-cephfs build-reva

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build-revad
build-revad: imports
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad

.PHONY: build-revad-cephfs
build-revad-cephfs: imports
	go build -ldflags ${BUILD_FLAGS} -tags ceph -o ./cmd/revad/revad ./cmd/revad

.PHONY: build-reva
build-reva: imports
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva

# to be run in Docker build
.PHONY: build-revad-docker
build-revad-docker: off
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad

.PHONY: build-revad-cephfs-docker
build-revad-cephfs-docker: off
	go build -ldflags ${BUILD_FLAGS} -tags ceph -o ./cmd/revad/revad ./cmd/revad

.PHONY: build-reva-docker
build-reva-docker: off
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva

.PHONY: test
test: off
	go test -coverprofile coverage.out -race $$(go list ./... | grep -v /tests/integration)

.PHONY: test-integration
test-integration: build-ci
	cd tests/integration && go test -race ./...

.PHONY: litmus-test-old
litmus-test-old: build
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c frontend.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c gateway.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-home.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-users.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c users.toml &
	docker run --rm --network=host -e LITMUS_URL=$(LITMUS_URL_OLD) -e LITMUS_USERNAME=$(LITMUS_USERNAME) -e LITMUS_PASSWORD=$(LITMUS_PASSWORD) -e TESTS=$(TESTS) owncloud/litmus:latest
	pkill revad

.PHONY: litmus-test-new
litmus-test-new: build
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c frontend.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c gateway.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-home.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-users.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c users.toml &
	docker run --rm --network=host -e LITMUS_URL=$(LITMUS_URL_NEW) -e LITMUS_USERNAME=$(LITMUS_USERNAME) -e LITMUS_PASSWORD=$(LITMUS_PASSWORD) -e TESTS=$(TESTS) owncloud/litmus:latest
	pkill revad

.PHONY: contrib
contrib:
	git shortlog -se | cut -c8- | sort -u | awk '{print "-", $$0}' | grep -v 'users.noreply.github.com' > CONTRIBUTORS.md

.PHONY: test-go-version
test-go-version:
	@echo -e "$(MINIMUM_GO_VERSION)\n$(shell echo $(GO_VERSION) | sed 's/go//')" | sort -Vc &> /dev/null || echo -e "\n\033[33;5m[WARNING]\033[0m You are not using a supported go version. Please use $(MINIMUM_GO_VERSION) or above.\n"

.PHONY: build-ci
build-ci: off
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva

.PHONY: gen-doc
gen-doc:
	go run tools/generate-documentation/main.go

.PHONY: clean
clean: toolchain-clean
	rm -rf dist

.PHONY: all
all: build test lint gen-doc

# create local build versions
dist: gen-doc
	go run tools/create-artifacts/main.go -version ${VERSION} -commit ${GIT_COMMIT} -goversion ${GO_VERSION}

.PHONY: mockery
mockery: $(MOCKERY)
	$(MOCKERY) --dir $(PWD) --output $(PWD)/mocks --boilerplate-file ./.templates/mockery.go.tmpl --name $(NAME)
	@echo ""

.PHONY: go-generate
go-generate:
	go generate ./...

BEHAT_BIN=vendor-bin/behat/vendor/bin/behat

test-acceptance-api: vendor-bin/behat/vendor
	BEHAT_BIN=$(BEHAT_BIN) $(PATH_TO_CORE)/tests/acceptance/run.sh --remote --type api

vendor/bamarni/composer-bin-plugin: composer.lock
	composer install

vendor-bin/behat/vendor: vendor/bamarni/composer-bin-plugin vendor-bin/behat/composer.lock
	composer bin behat install --no-progress

vendor-bin/behat/composer.lock: vendor-bin/behat/composer.json
	@echo behat composer.lock is not up to date.

composer.lock: composer.json
	@echo composer.lock is not up to date.