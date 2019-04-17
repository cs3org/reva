.PHONY: build
default: build test lint contrib

BUILD_DATE=`date +%FT%T%z`
GIT_COMMIT=`git rev-parse --short HEAD`
GIT_BRANCH=`git rev-parse --symbolic-full-name --abbrev-ref HEAD`
GIT_DIRTY=`git diff-index --quiet HEAD -- || echo "dirty-"`
VERSION=`git describe`

LDFLAGS=-ldflags "-s -X main.buildDate=${BUILD_DATE} -X main.gitCommit=${GIT_DIRTY}${GIT_COMMIT} -X main.gitBranch=${GIT_BRANCH} -X main.version=${VERSION}"

build:
	go build ./...
	go mod tidy
	go build -o ./cmd/revad/revad ${LDFLAGS} ./cmd/revad 

test:
	go test -race ./...

lint:
	go vet ./...
	golint -set_exit_status ./...
	go fmt ./...
	go run tools/check-license/check-license.go

contrib:
	git log --pretty="%an <%ae>" | sort -n | uniq  | sort -n | awk '{print "-", $$NA}' > CONTRIBUTORS.md
