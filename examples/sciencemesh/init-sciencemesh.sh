#!/usr/bin/env bash

set -e

# repositories and branches.
REPO_NEXTCLOUD_APP=https://github.com/MahdiBaghbani/nc-sciencemesh
BRANCH_NEXTCLOUD_APP=nextcloud

REPO_OWNCLOUD_APP=https://github.com/MahdiBaghbani/nc-sciencemesh
BRANCH_OWNCLOUD_APP=owncloud

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
