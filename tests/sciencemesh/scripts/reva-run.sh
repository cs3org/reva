#!/usr/bin/env bash

# create new dir and copy relevant configs there.
rm -rf /etc/revad
mkdir -p /etc/revad
if [ "${HOST::-1}" == "revacernbox" ]; then
  cp /configs/cernbox/* /etc/revad/
else
  cp /configs/revad/* /etc/revad/
fi

# substitute placeholders and "external" values with valid ones for the testnet.
sed -i "s/your.revad.ssl/${HOST}/g" /etc/revad/*.toml
sed -i "s/your.revad.org/${HOST}.docker/" /etc/revad/*.toml
sed -i "s/localhost/${HOST}.docker/" /etc/revad/*.toml
sed -i "s/your.efss.org/${HOST//reva/}.docker/" /etc/revad/sciencemesh.toml
sed -i "s/your.nginx.org/${HOST//reva/}.docker/" /etc/revad/cernbox.toml
sed -i "s/your.wopi.org/${HOST/reva/wopi/}.docker/" /etc/revad/*.toml
sed -i "/^mesh_directory_url /s/=.*$/= 'https:\/\/meshdir\.docker\/meshdir'/" /etc/revad/sciencemesh.toml
sed -i "/ocmproviderauthorizer\]/{n;s/.*/driver = \"json\"/;}" /etc/revad/sciencemesh.toml
sed -i "s/debug/trace/" /etc/revad/*.toml

cp /etc/tls/*.crt /usr/local/share/ca-certificates/
update-ca-certificates

# run revad.
revad --dev-dir "/etc/revad" &
