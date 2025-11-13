[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![GoDoc](https://godoc.org/github.com/cs3org/reva?status.svg)](https://godoc.org/github.com/cs3org/reva)
 [![Gitter chat](https://badges.gitter.im/cs3org/reva.svg)](https://gitter.im/cs3org/reva) [![Build Status](https://github.com/cs3org/reva/actions/workflows/docker.yml/badge.svg?branch=master&event=push)](https://github.com/cs3org/reva/actions)
 [![Go Report Card](https://goreportcard.com/badge/github.com/cs3org/reva)](https://goreportcard.com/report/github.com/cs3org/reva) [![FOSSA Status](https://app.fossa.com/api/projects/custom%2B11650%2Fcs3org%2Freva.svg?type=shield)](https://app.fossa.com/projects/custom%2B11650%2Fcs3org%2Freva?ref=badge_shield)
================

![Reva Logo](https://raw.githubusercontent.com/cs3org/logos/efd3d2649478193e74f3de5a41247445941026b6/reva/logo.jpg)

Reva is an interoperability platform consisting of several daemons written in Go.
It acts as bridge between high-level clients (mobile, web, desktop) and the underlying storage (CephFS, EOS, local filesytems). 
It exports well-known APIs, like WebDAV, to faciliate access from these devices.
It also exports a high-performance gRPC API, codenamed [CS3 APIs](https://buf.build/cs3org-buf/cs3apis), to easily integrate with other systems.
Reva is meant to be a high performant and customizable HTTP and gRPC server.

## Installation

Head to the [Releases](https://github.com/cs3org/reva/releases) to get the latest available release.

## Documentation & Support

Read the [beginners guide](https://github.com/cs3org/reva/wiki/beginners-guide) and the other featured guides in the [wiki](https://github.com/cs3org/reva/wiki). The wiki includes tutorials and guides contributed by the community.

In addition, at https://reva.link you can learn about the Reva project and its software components.

### Build and run it yourself

You need to have [Go](https://golang.org/doc/install) (version 1.21 or higher), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed. Some of these commands may require `sudo`, depending on your system setup.

```
# build
$ git clone https://github.com/cs3org/reva
$ cd reva
$ make revad
$ cmd/revad/revad --version
```

You can also read the [build from sources guide](https://github.com/cs3org/reva/wiki/build-reva) and the [setup tutorial](https://github.com/cs3org/reva/wiki/setup-tutorial).

### Run tests

To run unit tests do:
`make test-go`

To run gRPC integration tests do:
`make test-integration`
You can get more verbose output with `ginkgo -v -r tests/integration/`.

To run EOS tests you need to have an up and running Docker system:
`make docker-eos-full-tests`

## Versioning

The `master` branch is the stable development branch. All versions are tagged from it except for the `2.x` series: such versions were tagged out of a different branch codenamed `edge`, which now lives in external forks.

## Docker images

See [https://hub.docker.com/r/cs3org/reva](https://hub.docker.com/r/cs3org/reva).

## Plugin development
You can extend Reva without having to create PR's to this repo.
To do so, you can create plugins, pease checkout the [Wiki](https://github.com/cs3org/reva/wiki).

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

Reva's logo has been designed and contributed to the project by Eamonn Maguire.

## History

This project was initially conceived and brought to life by Hugo Gonzalez Labrador (@labkode) in 2017.
Since its roots, Reva has evolved and expanded thanks to the passion and commitment of 
dozens of remarkable individual contributors. [Learn more](https://reva.link/about/).

