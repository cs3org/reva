#!/bin/bash

set -e

git config --global --add safe.directory /reva
# go mod tidy
go mod vendor
make revad
make reva
