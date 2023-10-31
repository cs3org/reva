#!/usr/bin/env bash

set -e

# repositories and branches.
REPO_NEXTCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_NEXTCLOUD_APP=nextcloud

REPO_OWNCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_OWNCLOUD_APP=owncloud

# TODO will be dropped in favour of Reva directly serving the UI
CBOX_WEB=https://github.com/cernbox/web-release/releases/latest/download

REPO_WOPISERVER=https://github.com/cs3org/wopiserver
TAG_WOPISERVER=master

# TLS folder.
[ ! -d "tls" ] &&
    mkdir tls && cd scripts && ./gencerts.sh && cd -

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

# CERNBox web and extensions sources: uid=101 is nginx in the nginx container.
# TODO the extensions are temporarily extracted from a tgz
[ ! -d "cernbox-web-sciencemesh" ] &&                                           \
    mkdir -p temp/cernbox-1-conf temp/cernbox-2-conf &&                         \
    cp cernbox/nginx/* temp/cernbox-1-conf &&                                   \
    cp cernbox/nginx/* temp/cernbox-2-conf &&                                   \
    mkdir cernbox-web-sciencemesh &&                                            \
    cd cernbox-web-sciencemesh &&
    mkdir -p ./web && mkdir -p ./cernbox &&                                     \
    wget ${CBOX_WEB}/web.tar.gz &&                                              \
    tar xf web.tar.gz -C ./web --strip-components=1 &&                          \
    rm -rf web.tar.gz &&                                                        \
    tar xf ../cernbox/cernbox-extensions-bundle.tgz &&                          \
    chmod -R 755 ./* && chown -R 101:101 ./* &&                                 \
    cd -

# wopiserver source code for the config.
[ ! -d "wopi-sciencemesh" ] &&                                                       \
    git clone --branch ${TAG_WOPISERVER} ${REPO_WOPISERVER} wopi-sciencemesh &&      \
    mkdir -p temp/wopi-1-conf temp/wopi-2-conf &&                                    \
    cp wopi-sciencemesh/wopiserver.conf temp/wopi-1-conf/wopiserver.defaults.conf && \
    echo "shared-secret-2" > temp/wopi-1-conf/iopsecret &&                           \
    echo "wopisecret" > temp/wopi-1-conf/wopisecret &&                               \
    cp temp/wopi-1-conf/* temp/wopi-2-conf/
