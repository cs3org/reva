#!/usr/bin/env bash

set -e

# repositories and branches.
REPO_NEXTCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_NEXTCLOUD_APP=nextcloud

REPO_OWNCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_OWNCLOUD_APP=owncloud

REPO_REVA=https://github.com/cs3org/reva
BRANCH_REVA=sciencemesh-testing

# Nextcloud Sciencemesh source code.
[ ! -d "nc-sciencemesh" ] &&                                                    \
    git clone                                                                   \
    --branch ${BRANCH_NEXTCLOUD_APP}                                            \
    ${REPO_NEXTCLOUD_APP}                                                       \
    nc-sciencemesh                                                              \
    &&                                                                          \
    docker run -it                                                              \
    -v "$(pwd)/nc-sciencemesh:/var/www/html/apps/sciencemesh"                   \
    --workdir /var/www/html/apps/sciencemesh                                    \
    pondersource/dev-stock-nextcloud-sciencemesh                                \
    make composer

# ownCloud Sciencemesh source code.
[ ! -d "oc-sciencemesh" ] &&                                                    \
    git clone                                                                   \
    --branch ${BRANCH_OWNCLOUD_APP}                                             \
    ${REPO_OWNCLOUD_APP}                                                        \
    oc-sciencemesh                                                              \
    &&                                                                          \
    docker run -it                                                              \
    -v "$(pwd)/oc-sciencemesh:/var/www/html/apps/sciencemesh"                   \
    --workdir /var/www/html/apps/sciencemesh                                    \
    pondersource/dev-stock-owncloud-sciencemesh                                 \
    make composer

docker network inspect testnet >/dev/null 2>&1 || docker network create testnet

[ ! -d "temp" ] && mkdir --parents temp
