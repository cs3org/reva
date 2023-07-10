#!/usr/bin/env bash

# create new dir an compy configs there.
mkdir -p /revad/configs
cp /etc/revad/sciencemesh1.toml /revad/configs/sciencemesh1.toml
cp /etc/revad/sciencemesh2.toml /revad/configs/sciencemesh2.toml
cp /etc/revad/sciencemesh3.toml /revad/configs/sciencemesh3.toml
cp /etc/revad/providers.testnet.json /revad/configs/providers.testnet.json

# substitute placeholders with correct names.
sed -i "s/your.revad.ssl/${HOST}/g" /revad/configs/sciencemesh1.toml
sed -i "s/your.revad.com/${HOST}.docker/g" /revad/configs/sciencemesh1.toml
sed -i "s/your.efss.com/${HOST//reva/}.docker/g" /revad/configs/sciencemesh1.toml

sed -i "s/your.revad.ssl/${HOST}/g" /revad/configs/sciencemesh2.toml
sed -i "s/your.revad.com/${HOST}.docker/g" /revad/configs/sciencemesh2.toml
sed -i "s/your.efss.com/${HOST//reva/}.docker/g" /revad/configs/sciencemesh2.toml

sed -i "s/your.revad.ssl/${HOST}/g" /revad/configs/sciencemesh3.toml
sed -i "s/your.revad.com/${HOST}.docker/g" /revad/configs/sciencemesh3.toml
sed -i "s/your.efss.com/${HOST//reva/}.docker/g" /revad/configs/sciencemesh3.toml

# run revad.
revad --dev-dir "/revad/configs" -log "${LOG_LEVEL:-debug}" &
