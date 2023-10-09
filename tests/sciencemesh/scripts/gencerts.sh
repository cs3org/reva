#!/usr/bin/env bash
set -e

function createCert {
  echo "Generating key and CSR for ${1}.docker"
  openssl req -new -nodes \
    -out "../tls/${1}.csr" \
    -keyout "../tls/${1}.key" \
    -subj "/C=RO/ST=Bucharest/L=Bucharest/O=IT/CN=${1}.docker"
  echo Creating extfile
  echo "subjectAltName = @alt_names" > "../tls/${1}.cnf"
  echo "[alt_names]" >> "../tls/${1}.cnf"
  echo "DNS.1 = ${1}.docker" >> "../tls/${1}.cnf"

  echo "Signing CSR for ${1}.docker, creating cert."
  openssl x509 -req -days 3650 -in "../tls/${1}.csr" \
    -CA ../tls/ocm-ca.crt -CAkey ../tls/ocm-ca.key -CAcreateserial \
    -out "../tls/${1}.crt" -extfile "../tls/${1}.cnf"
}

rm --recursive --force ../tls
mkdir -p ../tls

echo "Generating CA key"
openssl genrsa -out ../tls/ocm-ca.key 2058

echo "Generate CA self-signed certificate"
openssl req -new -x509 -days 3650 \
    -key ../tls/ocm-ca.key \
    -out ../tls/ocm-ca.crt \
    -subj "/C=RO/ST=Bucharest/L=Bucharest/O=IT/CN=ocm-ca"

createCert meshdir
createCert stub1
createCert stub2
createCert nextcloud1
createCert nextcloud2
createCert owncloud1
createCert owncloud2
createCert revad1
createCert wopi1
createCert revad2
createCert wopi2
createCert revanextcloud1
createCert wopinextcloud1
createCert revanextcloud2
createCert wopinextcloud2
createCert revaowncloud1
createCert wopiowncloud1
createCert revaowncloud2
createCert wopiowncloud2
