BUILD_DATE	= `date +%FT%T%z`
GIT_COMMIT	?= `git rev-parse --short HEAD`
GIT_DIRTY	= `git diff-index --quiet HEAD -- || echo "dirty-"`
VERSION		?= `git describe --always`
GO_VERSION	?= `go version | awk '{print $$3}'`
BUILD_FLAGS	= "-X main.gitCommit=${GIT_COMMIT} -X main.version=${VERSION} -X main.goVersion=${GO_VERSION} -X main.buildDate=${BUILD_DATE}"

.PHONY: all
all: build-revad build-reva test lint gen-doc

IMAGE			?= revad:test

.PHONY: test-image
test-image:
	docker build -t $(IMAGE) -f docker/Dockerfile.revad .

LITMUS	?= $(CURDIR)/tests/litmus
TIMEOUT	?= 3600

.PHONY: litmus-only
litmus-only:
ifndef PROFILE
	$(error PROFILE is not defined)
else
	@cd $(LITMUS) && IMAGE=$(IMAGE) docker-compose --profile $(PROFILE) up --remove-orphans --exit-code-from litmus-$(PROFILE) --abort-on-container-exit --timeout $(TIMEOUT)
endif

.PHONY: litmus
litmus: test-image litmus-only

TOOLCHAIN		?= $(CURDIR)/toolchain
GOLANGCI_LINT	?= $(TOOLCHAIN)/golangci-lint
CALENS			?= $(TOOLCHAIN)/calens
GOIMPORTS		?= $(TOOLCHAIN)/goimports

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
	@$(GOLANGCI_LINT) run || (echo "Tip: many lint errors can be automatically fixed with \"make lint-fix\""; exit 1)

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
		grep -E '^ \* [[:alpha:]]{3} #$(PR): '
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
	go test $$([[ -z "${COVER_PROFILE}" ]] && echo "" || echo "-coverprofile=${COVER_PROFILE}") -race $$(go list ./... | grep -v /tests/integration)

.PHONY: test-integration
test-integration: build-revad
	go test -race ./tests/integration/...

.PHONY: contrib
contrib:
	git shortlog -se | cut -c8- | sort -u | awk '{print "-", $$0}' | grep -v 'users.noreply.github.com' > CONTRIBUTORS.md

.PHONY: gen-doc
gen-doc:
	go run tools/generate-documentation/main.go

.PHONY: clean
clean: toolchain-clean
	rm -rf dist

# create local build versions
dist: gen-doc
	go run tools/create-artifacts/main.go -version ${VERSION} -commit ${GIT_COMMIT} -goversion ${GO_VERSION}

BEHAT_BIN=vendor-bin/behat/vendor/bin/behat

test-acceptance-api: vendor-bin/behat/vendor
	BEHAT_BIN=$(BEHAT_BIN) $(PATH_TO_APITESTS)/tests/acceptance/run.sh --remote --type api

vendor/bamarni/composer-bin-plugin: composer.lock
	composer install

vendor-bin/behat/vendor: vendor/bamarni/composer-bin-plugin vendor-bin/behat/composer.lock
	composer bin behat install --no-progress

vendor-bin/behat/composer.lock: vendor-bin/behat/composer.json
	@echo behat composer.lock is not up to date.

composer.lock: composer.json
	@echo composer.lock is not up to date.