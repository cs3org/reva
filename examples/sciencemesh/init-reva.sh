#!/usr/bin/env bash

set -e
ENV_ROOT=$(pwd)
export ENV_ROOT=${ENV_ROOT}
[ ! -d "./scripts" ] && echo "Directory ./scripts DOES NOT exist inside $ENV_ROOT, are you running this from the repo root?" && exit 1

docker run --rm               -it    \
  -v "${ENV_ROOT}/../..:/reva"    \
  -v "${ENV_ROOT}/build-reva.sh:/build-reva.sh" \
  --workdir /reva                 \
  --entrypoint /bin/bash          \
  pondersource/dev-stock-revad    \
  /build-reva.sh

docker network inspect testnet >/dev/null 2>&1 || docker network create testnet

[ ! -d "temp" ] && mkdir --parents temp
