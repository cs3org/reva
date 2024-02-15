#!/usr/bin/env bash

set -e

# repositories and branches.
REPO_NEXTCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_NEXTCLOUD_APP=nextcloud

REPO_OWNCLOUD_APP=https://github.com/sciencemesh/nc-sciencemesh
BRANCH_OWNCLOUD_APP=owncloud

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

# CERNBox web bundle (temporary, to be served by Reva in the future):
# uid=101 is 'nginx' in the nginx container.
[ ! -d "cernbox-web-sciencemesh" ] &&
    mkdir cernbox-web-sciencemesh &&                                            \
    cd cernbox-web-sciencemesh &&                                               \
    tar xf ../cernbox/web-bundle.tgz &&                                         \
    cd web/js && sed -i "s|sciencemesh\.cesnet\.cz\/iop|meshdir\.docker|"       \
           web-app-science*mjs &&                                               \
    rm web-app-science*mjs.gz && gzip web-app-science*mjs &&                    \
    cd ../.. &&                                                                 \
    chmod -R 755 ./* && chown -R 101:101 ./* &&                                 \
    cd ..

# wopiserver source code for the config.
[ ! -d "wopi-sciencemesh" ] &&                                                  \
    git clone --branch ${TAG_WOPISERVER} ${REPO_WOPISERVER} wopi-sciencemesh    \

# Runtime configurations for WOPI and CERNBox.
[ ! -d "temp" ] &&                                                              \
    mkdir -p temp/cernbox-1-conf temp/cernbox-2-conf &&                         \
    cp cernbox/nginx/* temp/cernbox-1-conf &&                                   \
    cp cernbox/nginx/* temp/cernbox-2-conf &&                                   \
    mkdir -p temp/wopi-1-conf temp/wopi-2-conf &&                               \
    cp wopi-sciencemesh/wopiserver.conf                                         \
       temp/wopi-1-conf/wopiserver.defaults.conf &&                             \
    echo "shared-secret-2" > temp/wopi-1-conf/iopsecret &&                      \
    echo "wopisecret" > temp/wopi-1-conf/wopisecret &&                          \
    cp temp/wopi-1-conf/* temp/wopi-2-conf/ &&                                  \
    echo "temp folder for runtime configurations created"

