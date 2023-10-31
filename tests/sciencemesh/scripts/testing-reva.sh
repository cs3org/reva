#!/usr/bin/env bash

ENV_ROOT=$(pwd)
export ENV_ROOT=${ENV_ROOT}
[ ! -d "./scripts" ] && echo "Directory ./scripts DOES NOT exist inside $ENV_ROOT, are you running this from the repo root?" && exit 1

docker network inspect testnet >/dev/null 2>&1 || docker network create testnet

# make sure scripts are executable.
chmod +x "${ENV_ROOT}/scripts/reva-run.sh"
chmod +x "${ENV_ROOT}/scripts/reva-kill.sh"
chmod +x "${ENV_ROOT}/scripts/reva-entrypoint.sh"

# rclone
docker run --detach --name=rclone.docker --network=testnet  rclone/rclone rcd \                                   \
  -vv --rc-user=rcloneuser --rc-pass=eilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek \
  --rc-addr=0.0.0.0:5572 --server-side-across-configs=true                    \
  --log-file=/dev/stdout

# revad1
docker run --detach --network=testnet                                         \
  --name="revad1.docker"                                                      \
  -e HOST="revad1"                                                            \
  -v "${ENV_ROOT}/../..:/reva"                                                \
  -v "${ENV_ROOT}/revad:/configs/revad"                                       \
  -v "${ENV_ROOT}/tls:/etc/tls"                                               \
  -v "${ENV_ROOT}/scripts/reva-run.sh:/usr/bin/reva-run.sh"                   \
  -v "${ENV_ROOT}/scripts/reva-kill.sh:/usr/bin/reva-kill.sh"                 \
  -v "${ENV_ROOT}/scripts/reva-entrypoint.sh:/entrypoint.sh"                  \
  pondersource/dev-stock-revad "${REVA_CMD}"


# revad2
docker run --detach --network=testnet                                         \
  --name="revad2.docker"                                                      \
  -e HOST="revad2"                                                            \
  -v "${ENV_ROOT}/../..:/reva"                                                \
  -v "${ENV_ROOT}/revad:/configs/revad"                                       \
  -v "${ENV_ROOT}/tls:/etc/tls"                                               \
  -v "${ENV_ROOT}/scripts/reva-run.sh:/usr/bin/reva-run.sh"                   \
  -v "${ENV_ROOT}/scripts/reva-kill.sh:/usr/bin/reva-kill.sh"                 \
  -v "${ENV_ROOT}/scripts/reva-entrypoint.sh:/entrypoint.sh"                  \
  pondersource/dev-stock-revad "${REVA_CMD}"
