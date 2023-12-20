#!/bin/bash

set -e

git config --global --add safe.directory /reva
# go mod tidy
go mod vendor
#make gaia
#gaia build --with github.com/cernbox/reva-ocweb-plugin --with github.com/cs3org/reva=$(shell pwd) -o ./cmd/revad/revad
make revad
make reva
