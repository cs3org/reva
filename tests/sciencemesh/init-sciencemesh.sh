#!/usr/bin/env bash

set -e

# repositories and branches.
REPO_NEXTCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_NEXTCLOUD_APP=nextcloud

REPO_OWNCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_OWNCLOUD_APP=owncloud

REPO_WOPISERVER=https://github.com/cs3org/wopiserver
TAG_WOPISERVER=v10.2.0sm

# Nextcloud Sciencemesh source code.
[ ! -d "nextcloud-sciencemesh" ] &&                                             \
    git clone                                                                   \
    --branch ${BRANCH_NEXTCLOUD_APP}                                            \
    ${REPO_NEXTCLOUD_APP}                                                       \
    nextcloud-sciencemesh                                                       \
    &&                                                                          \
    docker run -it                                                              \
    -v "$(pwd)/nextcloud-sciencemesh:/var/www/html/apps/sciencemesh"            \
    --workdir /var/www/html/apps/sciencemesh                                    \
    pondersource/dev-stock-nextcloud-sciencemesh                                \
    make composer

# ownCloud Sciencemesh source code.
[ ! -d "owncloud-sciencemesh" ] &&                                              \
    git clone                                                                   \
    --branch ${BRANCH_OWNCLOUD_APP}                                             \
    ${REPO_OWNCLOUD_APP}                                                        \
    owncloud-sciencemesh                                                        \
    &&                                                                          \
    docker run -it                                                              \
    -v "$(pwd)/owncloud-sciencemesh:/var/www/html/apps/sciencemesh"             \
    --workdir /var/www/html/apps/sciencemesh                                    \
    pondersource/dev-stock-owncloud-sciencemesh                                 \
    composer install

# wopiserver source code for the config.
[ ! -d "wopiserver" ] &&                                                             \
    git clone --branch ${TAG_WOPISERVER} ${REPO_WOPISERVER} wopi-sciencemesh         \
    &&                                                                               \
    mkdir -p temp/wopi-1-conf temp/wopi-2-conf &&                                    \
    cp wopi-sciencemesh/docker/etc/*.cs3.conf temp/wopi-1-conf/wopiserver.conf &&    \
    cp wopi-sciencemesh/wopiserver.conf temp/wopi-1-conf/wopiserver.defaults.conf && \
    echo "shared-secret-2" > temp/wopi-1-conf/iopsecret &&                           \
    echo "wopisecret" > temp/wopi-1-conf/wopisecret &&                               \
    cp temp/wopi-1-conf/* temp/wopi-2-conf/
