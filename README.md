[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![GoDoc](https://godoc.org/github.com/cs3org/reva?status.svg)](https://godoc.org/github.com/cs3org/reva)
 [![Gitter chat](https://badges.gitter.im/cs3org/reva.svg)](https://gitter.im/cs3org/reva) [![Build Status](https://travis-ci.org/cs3org/reva.svg?branch=master)](https://travis-ci.org/cs3org/reva) [![Go Report Card](https://goreportcard.com/badge/github.com/cs3org/reva)](https://goreportcard.com/report/github.com/cs3org/reva)  [![codecov](https://codecov.io/gh/cs3org/reva/branch/master/graph/badge.svg)](https://codecov.io/gh/cs3org/reva) 
================




![REVA Logo](https://raw.githubusercontent.com/cs3org/logos/efd3d2649478193e74f3de5a41247445941026b6/reva/logo.jpg)

REVA is an interoperability platform. It connects storage, sync and share platforms and application providers, and it does it in a vendor and platform neutral way by using the [CS3 APIS](https://github.com/cs3org/cs3apis).

## Installation
Head to [Documentation](https://cs3org.github.io/reva/) for documentation or [download](https://github.com/cs3org/reva/releases) to get the latest release.

## Documentation & Support
Read the [getting started guide](https://cs3org.github.io/reva/beginner-guide.html) and the other feature guides.


## Build it yourself
You need to have [Go](https://golang.org/doc/install), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed.

```
$ git clone https://github.com/cs3org/reva
$ cd reva
$ make deps
$ make
$ cd cmd/revad
$ ./revad -c revad.toml -p revad.pid
```

You can also read the [build from sources guide](https://cs3org.github.io/reva/building-reva.html).

## Run it using Docker

```
$ git clone https://github.com/cs3org/reva
$ cd reva
$ docker build . -t revad
$ docker run -p 9999:9999 -p 9998:9998 -d revad
# or provide your own config 
# $ docker run -p 9999:9999 -p 9998:9998 -v /your/config:/etc/revad/revad.toml -d revad
```

## Plugin development

Checkout the [Plugin Development Guide](https://cs3org.github.io/reva/plugin-development.html).

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


[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fcs3org%2Freva.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fcs3org%2Freva?ref=badge_large)
