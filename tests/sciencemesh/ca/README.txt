This self-signed CA certificate has been generated with the following command:

echo "Generating CA key"
openssl genrsa -out ../tls/ocm-ca.key 2058

echo "Generate CA self-signed certificate"
openssl req -new -x509 -days 3650 \
    -key ../tls/ocm-ca.key \
    -out ../tls/ocm-ca.crt \
    -subj "/C=RO/ST=Bucharest/L=Bucharest/O=IT/CN=ocm-ca"

And the cert9.db file is a firefox certificate db that includes it, to be used in the
firefox container, for convenience and for correctly interacting with the keycloak IDP.
