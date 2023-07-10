#!/bin/bash

set -e

echo hi
git config --global --add safe.directory /reva
# go mod tidy
go mod vendor
make revad
make reva