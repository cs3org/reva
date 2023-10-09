#!/usr/bin/env bash

# create new dir and copy configs there.
mkdir -p /etc/wopi
cp /mnt/wopiconf/* /etc/wopi/

# substitute placeholders with correct values.
sed -i "s/your.wopi.org/${HOST}.docker/g" /etc/wopi/wopiserver.conf
sed -i "s/your.revad.org/${HOST/wopi/reva}.docker/g" /etc/wopi/wopiserver.conf

cp /etc/revad/tls/*.crt /usr/local/share/ca-certificates/
update-ca-certificates

# run wopiserver
python3 /app/wopiserver.py &
