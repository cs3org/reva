#!/usr/bin/env bash

set -e
ENV_ROOT=$(pwd)
export ENV_ROOT=${ENV_ROOT}

docker run --rm  -it                                    \
  -v "${ENV_ROOT}/../..:/reva"                          \
  -v "${ENV_ROOT}/scripts/build-reva.sh:/build-reva.sh" \
  --workdir /reva                                       \
  --entrypoint /bin/bash                                \
  pondersource/dev-stock-revad                          \
  /build-reva.sh

docker network inspect testnet >/dev/null 2>&1 || docker network create testnet

