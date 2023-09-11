#!/usr/bin/env bash

# create new dir and copy configs there.
mkdir -p /revad/configs
cp /etc/revad/sciencemesh*.toml /revad/configs/
cp /etc/revad/providers.testnet.json /revad/configs/providers.testnet.json

# substitute placeholders with correct names.
sed -i "s/your.revad.ssl/${HOST}/g" /revad/configs/sciencemesh*.toml
sed -i "s/your.revad.com/${HOST}.docker/g" /revad/configs/sciencemesh*.toml
sed -i "s/your.efss.com/${HOST//reva/}.docker/g" /revad/configs/sciencemesh*.toml

cp /etc/revad/tls/*.crt /usr/local/share/ca-certificates/
update-ca-certificates

# run revad.
revad --dev-dir "/revad/configs" &
