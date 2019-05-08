[REVA](https://cernbox.github.io/reva/) 
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0) [![GoDoc](https://godoc.org/github.com/cernbox/reva?status.svg)](https://godoc.org/github.com/cernbox/reva)
 [![Gitter chat](https://badges.gitter.im/cs3org/reva.svg)](https://gitter.im/cs3org/reva) [![Build Status](https://travis-ci.org/cernbox/reva.svg?branch=master)](https://travis-ci.org/cernbox/reva) [![Go Report Card](https://goreportcard.com/badge/github.com/cernbox/reva)](https://goreportcard.com/report/github.com/cernbox/reva)  [![codecov](https://codecov.io/gh/cernbox/reva/branch/master/graph/badge.svg)](https://codecov.io/gh/cernbox/reva) 
================
[Website](https://cernbox.github.io/reva/)

REVA is an interoperability platform. It connects storage, sync and share platforms and application providers, and it does it in a vendor and platform neutral way by using the [CS3 APIS](https://github.com/cernbox/cs3apis).

## Installation
Head to [Documentation](https://cernbox.github.io/reva/) for documentation or [download](https://github.com/cernbox/reva/releases) to get the latest release.

## Documentation & Support
Read the [getting started guide](https://cernbox.github.io/reva/beginner-guide.html) and the other feature guides.


## Build it yourself
You need to have [Go](https://golang.org/doc/install), [git](https://git-scm.com/) and [make](https://en.wikipedia.org/wiki/Make_(software)) installed.

```
$ git clone https://github.com/cernbox/reva
$ cd reva
$ make deps
$ make
$ cd cmd/revad
$ ./revad -c revad.toml -p revad.pid
```

You can also read the [build from sources guide](https://cernbox.github.io/reva/building-reva.html).

## Run it using Docker

```
$ git clone https://github.com/cernbox/reva
$ cd reva
$ docker build . -t revad
$ docker run -p 9999:9999 -p 9998:9998 -d revad
# or provide your own config 
# $ docker run -p 9999:9999 -p 9998:9998 -v /your/config:/etc/revad/revad.toml -d revad
```

## Plugin development

Checkout the [Plugin Development Guide](https://cernbox.github.io/reva/plugin-development.html).

## License

REVA is distributed under [Apache 2.0 license](https://github.com/cernbox/reva/blob/master/LICENSE).

## Disclaimer

There is no backward compatibility promises and semantic versioning yet.

If you want to use it, vendor it. It is always OK to change things to make things better.
The API is not 100% correct in the first commit.

Semantic versioning will be added once v1.0.0 is reached.
