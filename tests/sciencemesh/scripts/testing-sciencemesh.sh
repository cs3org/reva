#!/usr/bin/env bash

ENV_ROOT=$(pwd)
export ENV_ROOT=${ENV_ROOT}
[ ! -d "./scripts" ] && echo "Directory ./scripts DOES NOT exist inside $ENV_ROOT, are you running this from the repo root?" && exit 1
[ ! -d "./tls" ] && echo "Directory ./tls DOES NOT exist inside $ENV_ROOT, are you running this from the repo root?" && exit 1
[ ! -d "./nextcloud-sciencemesh" ] && echo "Directory ./nextcloud-sciencemesh DOES NOT exist inside $ENV_ROOT, did you run ./init.sh?" && exit 1
[ ! -d "./nextcloud-sciencemesh/vendor" ] && echo "Directory ./nextcloud-sciencemesh/vendor DOES NOT exist inside $ENV_ROOT. Try: rmdir ./nextcloud-sciencemesh ; ./scripts/init.sh" && exit 1
[ ! -d "./owncloud-sciencemesh" ] && echo "Directory ./owncloud-sciencemesh DOES NOT exist inside $ENV_ROOT, did you run ./init.sh?" && exit 1
[ ! -d "./owncloud-sciencemesh/vendor" ] && echo "Directory ./owncloud-sciencemesh/vendor DOES NOT exist inside $ENV_ROOT. Try: rmdir ./owncloud-sciencemesh ; ./scripts/init.sh" && exit 1
[ ! -d "./cernbox-web-sciencemesh" ] && echo "Directory ./cernbox-web-sciencemesh DOES NOT exist inside $ENV_ROOT, did you run ./init.sh?" && exit 1
[ ! -d "./wopi-sciencemesh" ] && echo "Directory ./wopi-sciencemesh DOES NOT exist inside $ENV_ROOT, did you run ./init.sh?" && exit 1

function waitForPort {
  x=$(docker exec -it "${1}" ss -tulpn | grep -c "${2}")
  until [ "${x}" -ne 0 ]
  do
    echo Waiting for "${1}" to open port "${2}", this usually takes about 10 seconds ... "${x}"
    sleep 1
    x=$(docker exec -it "${1}" ss -tulpn | grep -c "${2}")
  done
  echo "${1}" port "${2}" is open
}

function waitForCollabora {
  x=$(docker logs collabora.docker | grep -c "Ready")
  until [ "${x}" -ne 0 ]
  do
    echo Waiting for Collabora to be ready, this usually takes about 10 seconds ... "${x}"
    sleep 1
    x=$(docker logs collabora.docker | grep -c "Ready")
  done
  echo "Collabora is ready"
}

# create temp dirctory if it doesn't exist.
[ ! -d "${ENV_ROOT}/temp" ] && mkdir --parents "${ENV_ROOT}/temp"

# copy init files.
cp --force ./scripts/init-owncloud-sciencemesh.sh  ./temp/owncloud.sh
cp --force ./scripts/init-nextcloud-sciencemesh.sh ./temp/nextcloud.sh

# TLS dirs for mounting
[ ! -d "${ENV_ROOT}/${EFSS1}-1-tls" ] && cp --recursive --force ./tls "./temp/${EFSS1}-1-tls"
[ ! -d "${ENV_ROOT}/${EFSS2}-2-tls" ] && cp --recursive --force ./tls "./temp/${EFSS2}-2-tls"

# make sure scripts are executable.
chmod +x "${ENV_ROOT}/scripts/reva-run.sh"
chmod +x "${ENV_ROOT}/scripts/reva-kill.sh"
chmod +x "${ENV_ROOT}/scripts/reva-entrypoint.sh"

docker run --detach --name=meshdir.docker   --network=testnet -v "${ENV_ROOT}/scripts/stub.js:/ocm-stub/stub.js" -v "${ENV_ROOT}/tls:/tls" pondersource/dev-stock-ocmstub
docker run --detach --name=firefox          --network=testnet -v "${ENV_ROOT}/tls/cert9.db:/config/profile/cert9.db" -p 5800:5800 --shm-size 2g jlesage/firefox:latest

docker run --detach --name=collabora.docker --network=testnet -p 9980:9980 -t \
  -e "extra_params=--o:ssl.enable=false"                                      \
  -v "${ENV_ROOT}/collabora/coolwsd.xml:/etc/coolwsd/coolwsd.xml"             \
  -v "${ENV_ROOT}/tls:/tls"                                                   \
  collabora/code:latest 
#TODO the native container does not allow root shells, for now we disable SSL verification
#docker exec collabora.docker bash -c "cp /tls/*.crt /usr/local/share/ca-certificates/"
#docker exec collabora.docker update-ca-certificates

#docker run --detach --name=rclone.docker    --network=testnet                \
# rcd -vv --rc-addr=0.0.0.0:5572                                              \
# --rc-user=rcloneuser --rc-pass=eilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek     \
# --server-side-across-configs=true --log-file=/dev/stdout                    \
# rclone/rclone:latest

# this is used only by CERNBox so far, and might be used by OCIS in the future (though OCIS embeds an IDP)
docker run --detach --network=testnet --name=idp.docker                       \
  -e KEYCLOAK_ADMIN="admin" -e KEYCLOAK_ADMIN_PASSWORD="admin"                \
  -e KC_HOSTNAME="idp.docker"                                                 \
  -e KC_HTTPS_CERTIFICATE_FILE="/tls/idp.crt"                                 \
  -e KC_HTTPS_CERTIFICATE_KEY_FILE="/tls/idp.key"                             \
  -e KC_HTTPS_PORT="8443"                                                     \
  -v "${ENV_ROOT}/tls:/tls"                                                   \
  -v "${ENV_ROOT}/cernbox/keycloak:/opt/keycloak/data/import"                 \
  -p 8443:8443                                                                \
  quay.io/keycloak/keycloak:21.1.1                                            \
  start-dev --import-realm

# EFSS1
if [ "${EFSS1}" != "cernbox" ]; then

docker run --detach --network=testnet                                         \
  --name=maria1.docker                                                        \
  -e MARIADB_ROOT_PASSWORD=eilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek           \
  mariadb                                                                     \
  --transaction-isolation=READ-COMMITTED                                      \
  --binlog-format=ROW                                                         \
  --innodb-file-per-table=1                                                   \
  --skip-innodb-read-only-compressed

docker run --detach --network=testnet                                         \
  --name="${EFSS1}1.docker"                                                   \
  --add-host "host.docker.internal:host-gateway"                              \
  -e HOST="${EFSS1}1"                                                         \
  -e DBHOST="maria1.docker"                                                   \
  -e USER="einstein"                                                          \
  -e PASS="relativity"                                                        \
  -v "${ENV_ROOT}/temp/${EFSS1}.sh:/${EFSS1}-init.sh"                         \
  -v "${ENV_ROOT}/${EFSS1}-sciencemesh:/var/www/html/apps/sciencemesh"        \
  -v "${ENV_ROOT}/temp/${EFSS1}-1-tls:/tls"                                   \
  "pondersource/dev-stock-${EFSS1}-sciencemesh"

# setup
waitForPort maria1.docker 3306
waitForPort "${EFSS1}1.docker" 443

docker exec "${EFSS1}1.docker" bash -c "cp /tls/*.crt /usr/local/share/ca-certificates/"
docker exec "${EFSS1}1.docker" update-ca-certificates >& /dev/null
docker exec "${EFSS1}1.docker" bash -c "cat /etc/ssl/certs/ca-certificates.crt >> /var/www/html/resources/config/ca-bundle.crt"

docker exec -u www-data "${EFSS1}1.docker" sh "/${EFSS1}-init.sh"

# run db injections
docker exec maria1.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'iopUrl', 'https://reva${EFSS1}1.docker/');"

docker exec maria1.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'revaSharedSecret', 'shared-secret-1');"

docker exec maria1.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'meshDirectoryUrl', 'https://meshdir.docker/meshdir');"

docker exec maria1.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'inviteManagerApikey', 'invite-manager-endpoint');"

else

# setup only
sed < "${ENV_ROOT}/cernbox/nginx/nginx.conf"                                  \
  "s/your.revad.org/reva${EFSS1}1.docker/" |                                  \
  sed "s|your.cert.pem|/usr/local/share/ca-certificates/${EFSS1}1.crt|" |     \
  sed "s|your.key.pem|/usr/local/share/ca-certificates/${EFSS1}1.key|"        \
  > "${ENV_ROOT}/temp/cernbox-1-conf/nginx.conf"

sed < "${ENV_ROOT}/cernbox/web.json"                                          \
  "s/your.nginx.org/${EFSS1}1.docker/"                                        \
  > "${ENV_ROOT}/temp/cernbox-1-conf/config.json"

fi

# EFSS2
if [ "${EFSS2}" != "cernbox" ]; then

docker run --detach --network=testnet                                         \
  --name=maria2.docker                                                        \
  -e MARIADB_ROOT_PASSWORD=eilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek           \
  mariadb                                                                     \
  --transaction-isolation=READ-COMMITTED                                      \
  --binlog-format=ROW                                                         \
  --innodb-file-per-table=1                                                   \
  --skip-innodb-read-only-compressed

docker run --detach --network=testnet                                         \
  --name="${EFSS2}2.docker"                                                   \
  --add-host "host.docker.internal:host-gateway"                              \
  -e HOST="${EFSS2}2"                                                         \
  -e DBHOST="maria2.docker"                                                   \
  -e USER="marie"                                                             \
  -e PASS="radioactivity"                                                     \
  -v "${ENV_ROOT}/temp/${EFSS2}.sh:/${EFSS2}-init.sh"                         \
  -v "${ENV_ROOT}/${EFSS2}-sciencemesh:/var/www/html/apps/sciencemesh"        \
  -v "${ENV_ROOT}/temp/${EFSS2}-2-tls:/tls"                                   \
  "pondersource/dev-stock-${EFSS2}-sciencemesh"

# setup
waitForPort maria2.docker 3306
waitForPort "${EFSS2}2.docker" 443

docker exec "${EFSS2}2.docker" bash -c "cp /tls/*.crt /usr/local/share/ca-certificates/"
docker exec "${EFSS2}2.docker" update-ca-certificates >& /dev/null
docker exec "${EFSS2}2.docker" bash -c "cat /etc/ssl/certs/ca-certificates.crt >> /var/www/html/resources/config/ca-bundle.crt"

docker exec -u www-data "${EFSS2}2.docker" sh "/${EFSS2}-init.sh"

docker exec maria2.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'iopUrl', 'https://reva${EFSS2}2.docker/');"

docker exec maria2.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'revaSharedSecret', 'shared-secret-1');"

docker exec maria2.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'meshDirectoryUrl', 'https://meshdir.docker/meshdir');"

docker exec maria2.docker mariadb -u root -peilohtho9oTahsuongeeTh7reedahPo1Ohwi3aek efss                                                               \
  -e "insert into oc_appconfig (appid, configkey, configvalue) values ('sciencemesh', 'inviteManagerApikey', 'invite-manager-endpoint');"

else

# setup only
sed < "${ENV_ROOT}/cernbox/nginx/nginx.conf"                                  \
  "s/your.revad.org/reva${EFSS2}2.docker/" |                                  \
  sed "s|your.cert.pem|/usr/local/share/ca-certificates/${EFSS2}2.crt|" |     \
  sed "s|your.key.pem|/usr/local/share/ca-certificates/${EFSS2}2.key|"        \
  > "${ENV_ROOT}/temp/cernbox-2-conf/nginx.conf"

sed < "${ENV_ROOT}/cernbox/web.json"                                          \
  "s/your.nginx.org/${EFSS2}2.docker/"                                        \
  > "${ENV_ROOT}/temp/cernbox-2-conf/config.json"

fi

# IOP: reva
waitForCollabora
docker run --detach --network=testnet                                         \
  --name="reva${EFSS1}1.docker"                                               \
  -e HOST="reva${EFSS1}1"                                                     \
  -p 8080:80                                                                  \
  -v "${ENV_ROOT}/../..:/reva"                                                \
  -v "${ENV_ROOT}/revad:/configs/revad"                                       \
  -v "${ENV_ROOT}/cernbox:/configs/cernbox"                                   \
  -v "${ENV_ROOT}/temp/${EFSS1}-1-tls:/etc/tls"                               \
  -v "${ENV_ROOT}/scripts/reva-run.sh:/usr/bin/reva-run.sh"                   \
  -v "${ENV_ROOT}/scripts/reva-kill.sh:/usr/bin/reva-kill.sh"                 \
  -v "${ENV_ROOT}/scripts/reva-entrypoint.sh:/entrypoint.sh"                  \
  pondersource/dev-stock-revad

docker run --detach --network=testnet                                         \
  --name="reva${EFSS2}2.docker"                                               \
  -e HOST="reva${EFSS2}2"                                                     \
  -p 8180:80                                                                  \
  -v "${ENV_ROOT}/../..:/reva"                                                \
  -v "${ENV_ROOT}/revad:/configs/revad"                                       \
  -v "${ENV_ROOT}/cernbox:/configs/cernbox"                                   \
  -v "${ENV_ROOT}/temp/${EFSS2}-2-tls:/etc/tls"                               \
  -v "${ENV_ROOT}/scripts/reva-run.sh:/usr/bin/reva-run.sh"                   \
  -v "${ENV_ROOT}/scripts/reva-kill.sh:/usr/bin/reva-kill.sh"                 \
  -v "${ENV_ROOT}/scripts/reva-entrypoint.sh:/entrypoint.sh"                  \
  pondersource/dev-stock-revad

# IOP: wopi
sed < "${ENV_ROOT}/wopi-sciencemesh/docker/etc/wopiserver.cs3.conf"           \
  "s/your.wopi.org/wopi${EFSS1}1.docker/g" |                                  \
  sed "s/your.revad.org/reva${EFSS1}1.docker/g" |                             \
  sed "s|your.cert.pem|/usr/local/share/ca-certificates/wopi${EFSS1}1.crt|" | \
  sed "s|your.key.pem|/usr/local/share/ca-certificates/wopi${EFSS1}1.key|"    \
  > "${ENV_ROOT}/temp/wopi-1-conf/wopiserver.conf"

docker run --detach --network=testnet                                         \
  --name="wopi${EFSS1}1.docker"                                               \
  -e HOST="wopi${EFSS1}1"                                                     \
  -p 8880:8880                                                                \
  -v "${ENV_ROOT}/temp/wopi-1-conf:/etc/wopi"                                 \
  -v "${ENV_ROOT}/tls:/usr/local/share/ca-certificates"                       \
  cs3org/wopiserver:latest

docker exec "wopi${EFSS1}1.docker" update-ca-certificates >& /dev/null

sed < "${ENV_ROOT}/wopi-sciencemesh/docker/etc/wopiserver.cs3.conf"           \
  "s/your.wopi.org/wopi${EFSS2}2.docker/g" |                                  \
  sed "s/your.revad.org/reva${EFSS2}2.docker/g" |                             \
  sed "s|your.cert.pem|/usr/local/share/ca-certificates/wopi${EFSS2}2.crt|" | \
  sed "s|your.key.pem|/usr/local/share/ca-certificates/wopi${EFSS2}2.key|"    \
  > "${ENV_ROOT}/temp/wopi-2-conf/wopiserver.conf"

docker run --detach --network=testnet                                         \
  --name="wopi${EFSS2}2.docker"                                               \
  -e HOST="wopi${EFSS2}2"                                                     \
  -p 8980:8880                                                                \
  -v "${ENV_ROOT}/temp/wopi-2-conf:/etc/wopi"                                 \
  -v "${ENV_ROOT}/tls:/usr/local/share/ca-certificates"                       \
  cs3org/wopiserver:latest

docker exec "wopi${EFSS2}2.docker" update-ca-certificates >& /dev/null

# nginx for CERNBox, after reva
if [ "${EFSS1}" == "cernbox" ]; then

docker run --detach --network=testnet                                         \
  --name="${EFSS1}1.docker"                                                   \
  -v "${ENV_ROOT}/temp/cernbox-1-conf:/etc/nginx"                             \
  -v "${ENV_ROOT}/temp/cernbox-1-conf/config.json:/var/www/web/config.json"   \
  -v "${ENV_ROOT}/tls:/usr/local/share/ca-certificates"                       \
  -v "${ENV_ROOT}/cernbox-web-sciencemesh/web:/var/www/web"                   \
  -v "${ENV_ROOT}/cernbox-web-sciencemesh/cernbox:/var/www/cernbox"           \
  nginx

docker exec "${EFSS1}1.docker" update-ca-certificates >& /dev/null

fi

if [ "${EFSS2}" == "cernbox" ]; then

docker run --detach --network=testnet                                         \
  --name="${EFSS2}2.docker"                                                   \
  -v "${ENV_ROOT}/temp/cernbox-2-conf:/etc/nginx"                             \
  -v "${ENV_ROOT}/temp/cernbox-2-conf/config.json:/var/www/web/config.json"   \
  -v "${ENV_ROOT}/tls:/usr/local/share/ca-certificates"                       \
  -v "${ENV_ROOT}/cernbox-web-sciencemesh/web:/var/www/web"                   \
  -v "${ENV_ROOT}/cernbox-web-sciencemesh/cernbox:/var/www/cernbox"           \
  nginx

docker exec "${EFSS2}2.docker" update-ca-certificates >& /dev/null

fi


# instructions.
echo "Now browse to http://localhost:5800 and inside there to https://${EFSS1}1.docker"
echo "Log in as einstein / relativity"
echo "Go to the ScienceMesh app and generate a token"
echo "Click it to go to the meshdir server, and choose ${EFSS2}2 there."
echo "Log in on https://${EFSS2}2.docker as marie / radioactivity"
