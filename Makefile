.PHONY: build
default: build test lint contrib

BUILD_DATE=`date +%FT%T%z`
GIT_COMMIT=`git rev-parse --short HEAD`
GIT_BRANCH=`git rev-parse --symbolic-full-name --abbrev-ref HEAD`
GIT_DIRTY=`git diff-index --quiet HEAD -- || echo "dirty-"`
VERSION=`git describe --always`
GO_VERSION=`go version | awk '{print $$3}'`
BUILD_PLATFORM=`go version | awk '{print $$4}'`

LDFLAGS=-ldflags "-s -X main.buildDate=${BUILD_DATE} -X main.gitCommit=${GIT_DIRTY}${GIT_COMMIT} -X main.gitBranch=${GIT_BRANCH} -X main.version=${VERSION} -X main.goVersion=${GO_VERSION} -X main.buildPlatform=${BUILD_PLATFORM}"

build:
	GOPROXY=off
	go build -mod=vendor -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 
	go build -mod=vendor -o ./cmd/reva/reva ${LDFLAGS} ./cmd/reva

tidy:
	go mod tidy

build-revad:
	GOPROXY=off
	go build -mod=venodr -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 

build-reva:
	GOPROXY=off
	go build -mod=vendor -o ./cmd/reva/reva ${LDFLAGS} ./cmd/reva
	
test:
	GOPROXY=off
	go test -mod=vendor ./...

test-race:
	GOPROXY=off
	go test -mod=vendor -race ./...

lint:
	go run tools/check-license/check-license.go
	goimports -w tools pkg internal cmd

contrib:
	git log --pretty="%an <%ae>" | sort -n | uniq  | sort -n | awk '{print "-", $$0}' | grep -v 'users.noreply.github.com' > CONTRIBUTORS.md 

# for manual building
deps: 
	cd /tmp && go get -u golang.org/x/tools/cmd/goimports

# to be run in CI platform
ci: build test-race lint
