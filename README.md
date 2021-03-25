[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![GoDoc](https://godoc.org/github.com/cs3org/reva?status.svg)](https://godoc.org/github.com/cs3org/reva)
 [![Gitter chat](https://badges.gitter.im/cs3org/reva.svg)](https://gitter.im/cs3org/reva) [![Build Status](https://drone.cernbox.cern.ch/api/badges/cs3org/reva/status.svg)](https://drone.cernbox.cern.ch/cs3org/reva)
 [![Go Report Card](https://goreportcard.com/badge/github.com/cs3org/reva)](https://goreportcard.com/report/github.com/cs3org/reva)  [![codecov](https://codecov.io/gh/cs3org/reva/branch/master/graph/badge.svg)](https://codecov.io/gh/cs3org/reva) [![FOSSA Status](https://app.fossa.com/api/projects/custom%2B11650%2Fcs3org%2Freva.svg?type=shield)](https://app.fossa.com/projects/custom%2B11650%2Fcs3org%2Freva?ref=badge_shield)
================




![REVA Logo](https://raw.githubusercontent.com/cs3org/logos/efd3d2649478193e74f3de5a41247445941026b6/reva/logo.jpg)

REVA is an interoperability platform. It connects storage, sync and share platforms and application providers, and it does it in a vendor and platform neutral way by using the [CS3 APIS](https://github.com/cs3org/cs3apis).

## Installation
Head to [Documentation](https://reva.link) for documentation or [download](https://github.com/cs3org/reva/releases) to get the latest available release.

## Documentation & Support
Read the [getting started guide](https://reva.link/docs/getting-started/) and the other feature guides.


## Build and run it yourself
You need to have [Go](https://golang.org/doc/install), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed.

```
$ git clone https://github.com/cs3org/reva
$ cd reva
$ make deps
$ make
$ cd examples/storage-references
$ ../../cmd/revad/revad -dev-dir .
```

You can also read the [build from sources guide](https://reva.link/docs/getting-started/build-reva/).

## Run tests

### unit tests / GRPC tests
`make test`

### litmus tests
1. start the needed services
   ```
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

### ownCloud legacy integration tests
1. start an LDAP server
    ```
    docker run --rm --hostname ldap.my-company.com \
        -e LDAP_TLS_VERIFY_CLIENT=never \
        -e LDAP_DOMAIN=owncloud.com \
        -e LDAP_ORGANISATION=ownCloud \
        -e LDAP_ADMIN_PASSWORD=admin \
        --name docker-slapd \
        -p 127.0.0.1:389:389 \
        -p 636:636 -d osixia/openldap
    ```
2. start a REDIS server
    ```
    docker run --rm -e REDIS_DATABASES=1 -p 6379:6379 -d webhippie/redis:latest
    ```
3. start the needed services
    ```
    cd tests/oc-integration-tests/local
    ../../../cmd/revad/revad -c frontend.toml &
    ../../../cmd/revad/revad -c gateway.toml &
    ../../../cmd/revad/revad -c shares.toml &
    ../../../cmd/revad/revad -c storage-home.toml &
    ../../../cmd/revad/revad -c storage-users.toml &
    ../../../cmd/revad/revad -c storage-publiclink.toml &
    ../../../cmd/revad/revad -c ldap-users.toml
    ```

4. clone ownCloud 10
    `git clone https://github.com/owncloud/core.git ./testrunner`

5. clone the testing app
    `git clone https://github.com/owncloud/testing.git ./testrunner/apps/testing`

6. run the tests
    ```
    cd testrunner
    TEST_SERVER_URL='http://localhost:20080' \
    OCIS_REVA_DATA_ROOT='/var/tmp/reva/' \
    SKELETON_DIR='./apps/testing/data/apiSkeleton' \
    TEST_WITH_LDAP='true' \
    REVA_LDAP_HOSTNAME='localhost' \
    TEST_REVA='true' \
    BEHAT_FILTER_TAGS='~@skipOnOcis&&~@skipOnOcis-OC-Storage' \
    make test-acceptance-api
    ```

    This will run all tests that are relevant to reva.

    To run a single test add BEHAT_FEATURE=<feature file> and specify the path to the feature file and an optional line number. For example: BEHAT_FEATURE='tests/acceptance/features/apiWebdavUpload1/uploadFile.feature:12'

## Daily releases
On every commit on the master branch (including merged Pull Requests) a new release will be created and 
available at [daily releases](https://reva-releases.web.cern.ch/reva-releases).

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
