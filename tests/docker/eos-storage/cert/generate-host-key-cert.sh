#!/bin/sh

NODE=$1

ROOT_CA_CRT=MyOrg-RootCA.pem
ROOT_CA_KEY=MyOrg-RootCA.key

days=`echo $(( ($(date --date="20961212" +%s) - $(date +%s) )/(60*60*24) ))`

openssl genrsa -out $NODE.key 2048
openssl req -new -key $NODE.key -out $NODE.csr -subj "/CN=$NODE/C=IT/ST=Rome/L=Italy/O=$NODE"

cat > $NODE.ext <<EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = $NODE.test
EOF

openssl x509 -req -in $NODE.csr -CA $ROOT_CA_CRT -CAkey $ROOT_CA_KEY -CAcreateserial -out $NODE.crt -days $days -sha256 -extfile $NODE.ext
openssl x509 -in $NODE.crt -out ${NODE}cert.pem -outform PEM
mv $NODE.key ${NODE}key.pem

rm -f $NODE.{crt,csr,ext}