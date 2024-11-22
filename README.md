[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![GoDoc](https://godoc.org/github.com/cs3org/reva?status.svg)](https://godoc.org/github.com/cs3org/reva)
[![Gitter chat](https://badges.gitter.im/cs3org/reva.svg)](https://gitter.im/cs3org/reva) [![Build Status](https://drone.cernbox.cern.ch/api/badges/cs3org/reva/status.svg)](https://drone.cernbox.cern.ch/cs3org/reva)
[![Go Report Card](https://goreportcard.com/badge/github.com/cs3org/reva)](https://goreportcard.com/report/github.com/cs3org/reva) [![codecov](https://codecov.io/gh/cs3org/reva/branch/master/graph/badge.svg)](https://codecov.io/gh/cs3org/reva) [![FOSSA Status](https://app.fossa.com/api/projects/custom%2B11650%2Fcs3org%2Freva.svg?type=shield)](https://app.fossa.com/projects/custom%2B11650%2Fcs3org%2Freva?ref=badge_shield)
================

![REVA Logo](https://raw.githubusercontent.com/cs3org/logos/efd3d2649478193e74f3de5a41247445941026b6/reva/logo.jpg)

REVA is an interoperability platform. It connects storage, sync and share platforms and application providers, and it does it in a vendor and platform neutral way by using the [CS3 APIS](https://github.com/cs3org/cs3apis).

## Installation

Head to [Documentation](https://reva.link) for documentation or [download](https://github.com/cs3org/reva/releases) to get the latest available release.

## Documentation & Support

Read the [getting started guide](https://reva.link/docs/getting-started/) and the other feature guides.

## Build and run it yourself

You need to have [Go](https://golang.org/doc/install) (version 1.16 or higher), [git](https://git-scm.com/) and [make](<https://en.wikipedia.org/wiki/Make_(software)>) installed. Some of these commands may require `sudo`, depending on your system setup.

#### Build

```
$ git clone -b edge https://github.com/cs3org/reva
$ cd reva
$ make build
```

#### Run

```
$ mkdir -p /etc/revad
$ cp examples/storage-references/users.demo.json /etc/revad/users.json
$ cp examples/storage-references/groups.demo.json /etc/revad/groups.json
$ cp examples/storage-references/providers.demo.json /etc/revad/ocm-providers.json
$ cmd/revad/revad -dev-dir examples/storage-references

```

You can also read the [build from sources guide](https://reva.link/docs/getting-started/build-reva/) and the [setup tutorial](https://github.com/cs3org/reva/blob/master/docs/content/en/docs/tutorials/setup-tutorial.md).

## Run tests

Reva's codebase continuously undergoes testing at various levels.

To understand which tests exist, you can have a look at the [Makefile](https://github.com/cs3org/reva/blob/master/Makefile) and the [Drone run logs](https://drone.cernbox.cern.ch/cs3org/reva/).

The tests run by CERN's instance of [Drone CI/CD](https://docs.drone.io/) are defined in the [.drone.star](https://github.com/cs3org/reva/blob/master/.drone.star) file.

NB: The [tests/oc-integration-tests/drone](https://github.com/cs3org/reva/tree/master/tests/oc-integration-tests/drone) and [tests/oc-integration-tests/local](https://github.com/cs3org/reva/tree/master/tests/oc-integration-tests/local) folders contain the configuration fixtures that are used to start up the Reva instance to test (on drone CI/CD or on your local system, respectively), for both these acceptance tests ("ownCloud legacy integration tests") and the Litmus tests.

### Unit tests

This runs the `<unit>_test.go` files that appear next to some of the `<unit>.go` files in the code tree.

For instance `pkg/utils/utils_test.go` contains unit tests for `pkg/utils/utils.go`.

To run all of them you can do `make test`.

If you see `TestGetManagerWithInvalidUser/Nil_in_user` fail, [try removing](https://github.com/cs3org/reva/issues/1736) `/etc/revad/users.json` on your system.

To run a single one of them you can do:

```sh
$ go test `go list ./pkg/utils/...`
ok  	github.com/cs3org/reva/v2/pkg/utils	0.374s
```

### Integration tests (GRPC)

See [tests/integration](https://github.com/cs3org/reva/tree/master/tests/integration).
This requires Redis.

```sh
export REDIS_ADDRESS=127.0.0.1:6379
make test-integration
```

You can get more verbose output with `ginkgo -v -r tests/integration/`.

NB: This will work better on Linux than on MacOS because of issues with static linking (`library not found for -lcrt0.o`).

### Litmus tests (WebDAV)

[Litmus](http://www.webdav.org/neon/litmus/) is a webdav test suite. The litmus tests for Reva's WebDAV interface are run using the [ownCloud's litmus Docker image](https://github.com/owncloud-docker/litmus). The '-old' and '-new' refer to which `LITMUS_URL` environment variable is passed to that Docker image, in other words, which path on the Reva server the litmus tests are run against.

1. start the needed services

   ```
   mkdir -p /var/tmp/reva/einstein
   cd tests/oc-integration-tests/local
   ../../../cmd/revad/revad -c frontend.toml &
   ../../../cmd/revad/revad -c gateway.toml &
   ../../../cmd/revad/revad -c storage-home.toml &
   ../../../cmd/revad/revad -c storage-users.toml &
   ../../../cmd/revad/revad -c users.toml
   ```

2. run litmus tests:

   ```
   docker run --rm --network=host\
       -e LITMUS_URL=http://localhost:20080/remote.php/webdav \
       -e LITMUS_USERNAME=einstein \
       -e LITMUS_PASSWORD=relativity \
       owncloud/litmus:latest
   ```

   - add `TESTS` env. variable to test only a subset of tests e.g `-e TESTS="basic http copymove props"`
   - change `LITMUS_URL` for other tests e.g. `-e LITMUS_URL=http://localhost:20080/remote.php/dav/files/einstein` or to a public-share link
   - if on MacOS you see `FAIL (connection refused by '127.0.0.1' port 20080: Connection refused)`, it may be necessary to replace 'localhost' with your host IP address (e.g. `ipconfig getifaddr en0` or `sudo ifconfig | grep 192`)

### Acceptance tests (ownCloud legacy)

See [Running Acceptance Tests](https://github.com/cs3org/reva/tree/edge/tests/acceptance).

## Daily releases

On every commit on the master branch (including merged Pull Requests) a new release will be created and
available at [daily releases](https://reva-releases.web.cern.ch/reva-releases).

## Major versions

There are currently two major versions in active development.

### 1.x versions

The `master` branch is the stable development branch. Releases from master are tagged as 1.x.x versions following [semver](https://semver.org/).

### 2.x versions

The `edge` branch is used to develop the next version of this project. The edge branch is based on a new concept named "Spaces" and a new set of the CS3APIs are being implemented, making it **not compatible** with `master` branch. Releases from `edge` are tagged as _2.x.x_ versions following [semver](https://semver.org/).

## Run it using Docker

See [https://hub.docker.com/r/cs3org/reva](https://hub.docker.com/r/cs3org/reva).

## Plugin development

Checkout the [Tutorials](https://reva.link/docs/tutorials/).

## License

To promote free and unrestricted adoption of [CS3 APIs](https://github.com/cs3org/cs3apis) and the reference
implementation [REVA](https://github.com/cs3org/reva) by all EFSS implementations and all platforms and
application providers, both community and commercial, Open Source and
Open Core, [CERN](https://home.cern/) released the source code repositories under [Apache 2.0 license](https://github.com/cs3org/reva/blob/master/LICENSE).

Further evolution of the CS3 APIs will be driven by the needs of the
Educational and Research community with the goal of maximizing the
portability of the applications and service extensions.

REVA is distributed under [Apache 2.0 license](https://github.com/cs3org/reva/blob/master/LICENSE).

## Logo

REVA logo's have been designed and contributed to the project by Eamon Maguire.

## Disclaimer

There is no backward compatibility promises and semantic versioning yet.
Semantic versioning will be added once v1.0.0 is reached.

If you want to use it, vendor it. It is always OK to change things to make things better.
The API is not 100% correct in the first commit.
