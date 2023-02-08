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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | BINDIR=$(@D) sh -s v1.50.1

$(CALENS):
	@mkdir -p $(@D)
	git clone --depth 1 --branch v0.2.0 -c advice.detachedHead=false https://github.com/restic/calens.git /tmp/calens
	cd /tmp/calens && GOBIN=$(@D) go install
	rm -rf /tmp/calens


################################################################################
# Build
################################################################################

GIT_COMMIT	?= `git rev-parse --short HEAD`
VERSION		?= `git describe --always`
GO_VERSION	?= `go version | awk '{print $$3}'`
BUILD_DATE	= `date +%FT%T%z`
BUILD_FLAGS	= "-X main.gitCommit=$(GIT_COMMIT) -X main.version=$(VERSION) -X main.goVersion=$(GO_VERSION) -X main.buildDate=$(BUILD_DATE)"

.PHONY: revad
revad:
	go build -ldflags $(BUILD_FLAGS) -o ./cmd/revad/revad ./cmd/revad

.PHONY: revad-cephfs
revad-cephfs:
	go build -ldflags $(BUILD_FLAGS) -tags ceph -o ./cmd/revad/revad ./cmd/revad

.PHONY: reva
reva:
	go build -ldflags $(BUILD_FLAGS) -o ./cmd/reva/reva ./cmd/reva

################################################################################
# Test
################################################################################

REVAD_IMAGE	?= revad:test

.PHONY: test-docker
test-docker:
	docker build -f docker/Dockerfile.revad -t $(REVAD_IMAGE) .

TESTS		= litmus-1 litmus-2 litmus-3 acceptance-1
TIMEOUT		?= 3600

.PHONY: $(TESTS)
$(TESTS): test-docker
	docker pull cs3org/behat:latest
	REVAD_IMAGE=$(REVAD_IMAGE) \
	docker-compose --file tests/docker-compose/$@.yml --project-directory . \
		up --force-recreate --renew-anon-volumes --remove-orphans \
		--exit-code-from $@ --abort-on-container-exit --timeout $(TIMEOUT)

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

.PHONY: clean
clean: toolchain-clean
	rm -rf dist


test-acceptance-api:
	$(PATH_TO_APITESTS)/tests/acceptance/run.sh --type api
