#!/bin/bash

eos mkdir -p /eos/user

for letter in {a..z}; do
  eos mkdir -p "/eos/user/$letter"
done

eos vid set membership 0 +sudo
eos vid set membership 99 +sudo
eos vid set map -tident "*@storage-home" vuid:0 vgid:0
eos vid set map -tident "*@storage-users" vuid:0 vgid:0
eos vid set map -tident "*@storage-local-1" vuid:0 vgid:0
eos vid set map -tident "*@storage-local-2" vuid:0 vgid:0
