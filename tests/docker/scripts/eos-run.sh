#!/bin/bash

eos daemon sss recreate
eos daemon run mq &
eos daemon run qdb &
eos daemon run mgm &
eos daemon run fst &
sleep 5

for name in 01; do
  mkdir -p /data/fst/$$name;
  chown daemon:daemon /data/fst/$$name
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
eosxd -ofsname=`hostname -f`:/eos/ /eos/

eos mkdir -p /eos/user

for letter in {a..z}; do
  eos mkdir -p /eos/user/$letter
done

eos vid set membership 0 +sudo
eos vid set membership 99 +sudo
eos vid set map -tident "*@storage-home-ocis" vuid:0 vgid:0

tail -f /dev/null
