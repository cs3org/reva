#!/bin/bash

eos daemon sss recreate
eos daemon run mq &
eos daemon run qdb &
eos daemon run mgm &
eos daemon run fst &
sssd
sleep 5

for name in 01; do
  mkdir -p /data/fst/$name;
  chown daemon:daemon /data/fst/$name
done
eos space define default

eosfstregister -r localhost /data/fst/ default:1

eos space set default on
eos mkdir /eos/dev/rep-2/
eos mkdir /eos/dev/ec-42/
eos attr set default=replica /eos/dev/rep-2 /
eos attr set default=raid6 /eos/dev/ec-42/
eos chmod 777 /eos/dev/rep-2/
eos chmod 777 /eos/dev/ec-42/
mkdir -p /eos/
eosxd -ofsname=$(hostname -f):/eos/ /eos/

eos mkdir -p /eos/user

for letter in {a..z}; do
  eos mkdir -p "/eos/user/$letter"
done

# create cbox sudoer user
adduser cbox -u 58679 -g 0 -m -s /bin/sh
eos vid set membership 0 +sudo
eos vid set membership cbox +sudo

eos vid set map -tident "*@storage-home" vuid:58679 vgid:0
eos vid set map -tident "*@storage-users" vuid:58679 vgid:0
eos vid set map -tident "*@storage-local-1" vuid:58679 vgid:0
eos vid set map -tident "*@storage-local-2" vuid:58679 vgid:0
eos vid set map -tident "*@docker-storage-home-1.docker_default" vuid:58679 vgid:0

eos vid set map -tident "unix@storage-home" vuid:58679 vgid:0
eos vid set map -tident "unix@storage-users" vuid:58679 vgid:0
eos vid set map -tident "unix@storage-local-1" vuid:58679 vgid:0
eos vid set map -tident "unix@storage-local-2" vuid:58679 vgid:0
eos vid set map -tident "unix@docker-storage-home-1.docker_default" vuid:58679 vgid:0

tail -f /dev/null
