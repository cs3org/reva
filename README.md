[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![GoDoc](https://godoc.org/github.com/cs3org/reva?status.svg)](https://godoc.org/github.com/cs3org/reva)
 [![Gitter chat](https://badges.gitter.im/cs3org/reva.svg)](https://gitter.im/cs3org/reva) [![Build Status](https://github.com/cs3org/reva/actions/workflows/docker.yml/badge.svg?branch=master&event=push)](https://github.com/cs3org/reva/actions)
 [![Go Report Card](https://goreportcard.com/badge/github.com/cs3org/reva)](https://goreportcard.com/report/github.com/cs3org/reva) [![FOSSA Status](https://app.fossa.com/api/projects/custom%2B11650%2Fcs3org%2Freva.svg?type=shield)](https://app.fossa.com/projects/custom%2B11650%2Fcs3org%2Freva?ref=badge_shield)
================


![Reva Logo](https://raw.githubusercontent.com/cs3org/logos/efd3d2649478193e74f3de5a41247445941026b6/reva/logo.jpg)

Reva is an interoperability platform consisting of several daemons written in Go.
It acts as bridge between high-level clients (mobile, web, desktop) and the underlying storage (CephFS, EOS, local filesytems). 
It exports well-known APIs, like WebDAV, to faciliate access from these devices.
It also exports a high-performance gRPC API, codenamed [CS3APIS](https://buf.build/cs3org-buf/cs3apis), to easily integrate with other systems.
Reva is meant to be a high performant and customizable HTTP and GRPC server.

## Installation

Head to [Documentation](https://reva.link) for documentation or [download](https://github.com/cs3org/reva/releases) to get the latest available release.

## Documentation & Support

Read the [getting started guide](https://reva.link/docs/getting-started/) and the other feature guides.


## Contributing: Build and run it yourself

You need to have [Go](https://golang.org/doc/install) (version 1.21 or higher), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed. Some of these commands may require `sudo`, depending on your system setup.

```
# build
$ git clone https://github.com/cs3org/reva
$ cd reva
$ make revad
$ cmd/revad/revad --version
```

You can also read the [build from sources guide](https://reva.link/docs/getting-started/build-reva/) and the [setup tutorial](https://github.com/cs3org/reva/blob/master/docs/content/en/docs/tutorials/setup-tutorial.md).

## Contributing: Run tests

To run unit tests do:
`make test-go`

To run GRPC integration tests do:
`make test-integration`
You can get more verbose output with `ginkgo -v -r tests/integration/`.

To run EOS tests you need to have an up and running Docker system:
`make docker-eos-full-tests`

## Versioning

There are currently two major versions in active development.

### 1.x versions

The ``master`` branch is the stable development branch. Releases from master are tagged as 1.x.x versions following [semver](https://semver.org/).
Use this version for standalone deployment.

### 2.x versions

The ``edge`` branch is used as a dependency for ownCloud's OCIS product and differs from 1.X versions. Please do not use 2.X for standalone deployments and always use them as part of the OCIS product.

## Docker images

See [https://hub.docker.com/r/cs3org/reva](https://hub.docker.com/r/cs3org/reva).

## Plugin development
You can extend Reva without having to create PR's to this repo.
To do so, you can create plugins, pease checkout the [Tutorials](https://reva.link/docs/tutorials/).

## License

To promote free and unrestricted adoption of [CS3 APIs](https://github.com/cs3org/cs3apis) and the reference
implementation [Reva](https://github.com/cs3org/reva) by all EFSS implementations and all platforms and
application providers, both community and commercial, Open Source and
Open Core, [CERN](https://home.cern/) released the source code repositories under [Apache 2.0 license](https://github.com/cs3org/reva/blob/master/LICENSE).

Further evolution of the CS3 APIs will be driven by the needs of the
Educational and Research community with the goal of maximizing the
portability of the applications and service extensions.

Reva is distributed under [Apache 2.0 license](https://github.com/cs3org/reva/blob/master/LICENSE).

## Logo

Reva logo's have been designed and contributed to the project by Eamon Maguire.
