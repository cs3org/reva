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

off: 
	GORPOXY=off

imports: off
	goimports -w tools pkg internal cmd

build: imports
	go build -mod=vendor -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 
	go build -mod=vendor -o ./cmd/reva/reva ${LDFLAGS} ./cmd/reva
	
tidy:
	go mod tidy

build-revad: imports
	go build -mod=vendor -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 

build-reva: imports
	go build -mod=vendor -o ./cmd/reva/reva ${LDFLAGS} ./cmd/reva
	
test: off
	go test -mod=vendor -race ./... 

lint:
	go run tools/check-license/check-license.go
	golangci-lint run

contrib:
	git log --pretty="%an <%ae>" | sort -n | uniq  | sort -n | awk '{print "-", $$0}' | grep -v 'users.noreply.github.com' > CONTRIBUTORS.md 

# for manual building only
deps: 
	cd /tmp && go get github.com/golangci/golangci-lint/cmd/golangci-lint
	cd /tmp && go get golang.org/x/tools/cmd/goimports

build-ci: off
	go build -mod=vendor -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 
	go build -mod=vendor -o ./cmd/reva/reva ${LDFLAGS} ./cmd/reva
lint-ci:
	go run tools/check-license/check-license.go


# to be run in CI platform
ci: build-ci test  lint-ci

# to be run in Docker build
build-revad-docker: off
	go build -mod=vendor -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 
build-reva-docker: off
	go build -mod=vendor -o ./cmd/revad/reva ${LDFLAGS} ./cmd/reva
clean:
	rm -rf dist

# for releasing you need to run: go run tools/release/main.go
release-deps:
	cd /tmp && go get github.com/restic/calens

# usually to be run from CI, it will push artefacts to github releases
publish:
	rm -rf dist && mkdir dist
	GOOS=linux GOARCH=amd64 go build -mod=vendor -o ./dist/revad_${VERSION}_linux_amd64 ${LDFLAGS} ./cmd/revad
	GOOS=linux GOARCH=386   go build -mod=vendor -o ./dist/revad_${VERSION}_linux_386   ${LDFLAGS} ./cmd/revad
	GOOS=darwin GOARCH=amd64 go build -mod=vendor -o ./dist/revad_${VERSION}_darwin_amd64 ${LDFLAGS} ./cmd/revad
	GOOS=darwin GOARCH=386   go build -mod=vendor -o ./dist/revad_${VERSION}_darwin_386   ${LDFLAGS} ./cmd/revad
	cd dist && sha256sum * > checksums-sha256.txt
