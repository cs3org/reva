[![Go Report Card](https://goreportcard.com/badge/github.com/cernbox/reva)](https://goreportcard.com/report/github.com/cernbox/reva) [![Build Status](https://travis-ci.org/cernbox/reva.svg?branch=master)](https://travis-ci.org/cernbox/reva)

# REVA

Cloud Storage Sync & Share Interoperability Platform

# Getting started - Docker

```
git clone https://github.com/cernbox/reva
cd reva
docker build . -t revad
docker run -p 9999:9999 -p 9998:9998 -d revad
```

# Getting started - Docker with your own config

```
git clone https://github.com/cernbox/reva
cd reva
docker build . -t revad
docker run -p 9999:9999 -p 9998:9998 -v /your/config:/etc/revad/revad.toml -d revad
```

# Getting started - Docker Compose

```
git clone https://github.com/cernbox/reva
cd reva/examples/docker-compose/
docker-compose up -d
```
