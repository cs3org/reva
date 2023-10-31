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

echo "Copying CA certificate and CA browser db"
cp ../ca/* ../tls

createCert meshdir
createCert stub1
createCert stub2

createCert revad1
createCert revad2

createCert idp
chown 1000:root ../tls/idp.*

for efss in owncloud nextcloud cernbox; do
  createCert ${efss}1
  createCert ${efss}2
  createCert reva${efss}1
  createCert reva${efss}2
  createCert wopi${efss}1
  createCert wopi${efss}2
done
