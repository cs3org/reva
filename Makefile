.PHONY: build

SHELL := /bin/bash
BUILD_DATE=`date +%FT%T%z`
GIT_COMMIT=`git rev-parse --short HEAD`
GIT_BRANCH=`git rev-parse --symbolic-full-name --abbrev-ref HEAD`
GIT_DIRTY=`git diff-index --quiet HEAD -- || echo "dirty-"`
VERSION=`git describe --always`
GO_VERSION=`go version | awk '{print $$3}'`

default: build test lint contrib
release: deps build test lint

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
	go build -o ./cmd/revad/revad ./cmd/revad
	go build -o ./cmd/reva/reva ./cmd/reva

tidy:
	go mod tidy

build-revad: imports
	go build -o ./cmd/revad/revad ./cmd/revad

build-reva: imports
	go build -o ./cmd/reva/reva ./cmd/reva

test: off
	go test -race ./...

lint:
	go run tools/check-license/check-license.go
	`go env GOPATH`/bin/golangci-lint run

contrib:
	#git shortlog -se | cut -c8- | sort -u | awk '{print "-", $$0}' | grep -v 'users.noreply.github.com' > CONTRIBUTORS.md

# for manual building only
deps:
	cd /tmp && rm -rf golangci-lint &&  git clone --quiet -b 'v1.21.0' --single-branch --depth 1 https://github.com/golangci/golangci-lint &> /dev/null && cd golangci-lint/cmd/golangci-lint && go install
	cd /tmp && go get golang.org/x/tools/cmd/goimports

build-ci: off
	go build -o ./cmd/revad/revad ./cmd/revad
	go build -o ./cmd/reva/reva ./cmd/reva

lint-ci:
	go run tools/check-license/check-license.go


# to be run in CI platform
ci: build-ci test  lint-ci

# to be run in Docker build
build-revad-docker: off
	go build -o ./cmd/revad/revad ./cmd/revad
build-reva-docker: off
	go build -o ./cmd/reva/reva ./cmd/reva
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
