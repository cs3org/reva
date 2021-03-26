.PHONY: build

SHELL := /bin/bash
BUILD_DATE=`date +%FT%T%z`
GIT_COMMIT=`git rev-parse --short HEAD`
GIT_BRANCH=`git rev-parse --symbolic-full-name --abbrev-ref HEAD`
GIT_DIRTY=`git diff-index --quiet HEAD -- || echo "dirty-"`
VERSION=`git describe --always`
GO_VERSION=`go version | awk '{print $$3}'`
BUILD_FLAGS="-X main.gitCommit=${GIT_COMMIT} -X main.version=${VERSION} -X main.goVersion=${GO_VERSION} -X main.buildDate=${BUILD_DATE}"
LITMUS_URL_OLD="http://localhost:20080/remote.php/webdav"
LITMUS_URL_NEW="http://localhost:20080/remote.php/dav/files/4c510ada-c86b-4815-8820-42cdf82c3d51"
LITMUS_USERNAME="einstein"
LITMUS_PASSWORD="relativity"
TESTS="basic http copymove props"

default: build test lint gen-doc check-changelog
release: deps build test lint gen-doc

off:
	GOPROXY=off
	echo BUILD_DATE=${BUILD_DATE}
	echo GIT_COMMIT=${GIT_COMMIT}
	echo GIT_DIRTY=${GIT_DIRTY}
	echo VERSION=${VERSION}
	echo GO_VERSION=${GO_VERSION}

imports: off
	`go env GOPATH`/bin/goimports -w tools pkg internal cmd

build: imports
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva

tidy:
	go mod tidy

build-revad: imports
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad

build-reva: imports
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva

test: off
	go test -race $$(go list ./... | grep -v /tests/integration)

test-integration: build-ci
	cd tests/integration && go test -race ./...

litmus-test-old: build
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c frontend.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c gateway.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-home.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-users.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c users.toml &
	docker run --rm --network=host -e LITMUS_URL=$(LITMUS_URL_OLD) -e LITMUS_USERNAME=$(LITMUS_USERNAME) -e LITMUS_PASSWORD=$(LITMUS_PASSWORD) -e TESTS=$(TESTS) owncloud/litmus:latest
	pkill revad
litmus-test-new: build
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c frontend.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c gateway.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-home.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c storage-users.toml &
	cd tests/oc-integration-tests/local && ../../../cmd/revad/revad -c users.toml &
	docker run --rm --network=host -e LITMUS_URL=$(LITMUS_URL_NEW) -e LITMUS_USERNAME=$(LITMUS_USERNAME) -e LITMUS_PASSWORD=$(LITMUS_PASSWORD) -e TESTS=$(TESTS) owncloud/litmus:latest
	pkill revad
lint:
	go run tools/check-license/check-license.go
	`go env GOPATH`/bin/golangci-lint run --timeout 2m0s

contrib:
	git shortlog -se | cut -c8- | sort -u | awk '{print "-", $$0}' | grep -v 'users.noreply.github.com' > CONTRIBUTORS.md

# for manual building only
deps:
	cd /tmp && rm -rf golangci-lint &&  git clone --quiet -b 'v1.38.0' --single-branch --depth 1 https://github.com/golangci/golangci-lint &> /dev/null && cd golangci-lint/cmd/golangci-lint && go install
	cd /tmp && go get golang.org/x/tools/cmd/goimports

build-ci: off
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva

lint-ci:
	go run tools/check-license/check-license.go

gen-doc:
	go run tools/generate-documentation/main.go

check-changelog: release-deps
	`go env GOPATH`/bin/calens > /dev/null
	go run tools/check-changelog/main.go

check-changelog-drone:
	go run tools/check-changelog/main.go -repo origin -pr "$(PR)"

# to be run in CI platform
ci: build-ci test  lint-ci

# to be run in Docker build
build-revad-docker: off
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/revad/revad ./cmd/revad
build-reva-docker: off
	go build -ldflags ${BUILD_FLAGS} -o ./cmd/reva/reva ./cmd/reva
clean:
	rm -rf dist

# for releasing you need to run: go run tools/prepare-release/main.go
# $ go run tools/prepare-release/main.go -version 0.0.1 -commit -tag
release-deps:
	cd /tmp && rm -rf calens &&  git clone --quiet -b 'v0.2.0' --single-branch --depth 1 https://github.com/restic/calens &> /dev/null && cd calens && go install

# create local build versions
dist: default
	go run tools/create-artifacts/main.go -version ${VERSION} -commit ${GIT_COMMIT} -goversion ${GO_VERSION}

all: deps default

BEHAT_BIN=vendor-bin/behat/vendor/bin/behat

.PHONY: test-acceptance-api
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
