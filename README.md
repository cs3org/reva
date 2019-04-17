[REVA](https://cernbox.github.io/reva/) 
[![Go Report Card](https://goreportcard.com/badge/github.com/cernbox/reva)](https://goreportcard.com/report/github.com/cernbox/reva) [![Build Status](https://travis-ci.org/cernbox/reva.svg?branch=master)](https://travis-ci.org/cernbox/reva) [![codecov](https://codecov.io/gh/cernbox/reva/branch/master/graph/badge.svg)](https://codecov.io/gh/cernbox/reva)
================
[Website](https://cernbox.github.io/reva/)

REVA is an interoperability platform. It connects storage, sync and share platforms and application providers, and it does it in a vendor and platform neutral way by using the [CS3 APIS](https://github.com/cernbox/cs3apis).

## Installation
Head to [Documentation](https://cernbox.github.io/reva/) for documentation or [download](https://github.com/cernbox/reva/releases) to get the latest release.

## Documentation & Support
Read the [getting started guide](https://cernbox.github.io/reva/beginner-guide.html) and the other feature guides.


## Build it yourself
Read the [build from sources guide](https://cernbox.github.io/reva/building-reva.html).

## Run it using Docker

```
git clone https://github.com/cernbox/reva
cd reva
docker build . -t revad
docker run -p 9999:9999 -p 9998:9998 -d revad
# or provide your own config 
# docker run -p 9999:9999 -p 9998:9998 -v /your/config:/etc/revad/revad.toml -d revad
```

## Plugin development

Checkout the [Plugin Development Guide](https://cernbox.github.io/reva/plugin-development.html).

## License

REVA is distributed under [Apache 2.0 license](https://github.com/cernbox/reva/blob/master/LICENSE).

## Disclaimer

There is no backward compatibility promises and semantic versioning yet.
If you want to use it, vendor it. It is always OK to change things to make things better.
The API is not 100% correct in the first commit.
Once we reached version v1.0.0, semantic versioning will be added.
