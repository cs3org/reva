#!/bin/bash

eos mkdir -p /eos/user

for letter in {a..z}; do
  eos mkdir -p "/eos/user/$letter"
done

eos vid set membership 0 +sudo
eos vid set membership 99 +sudo

for prot in grpc https
do
  eos vid add gateway storage-home $prot
  eos vid add gateway storage-users $prot
  eos vid add gateway storage-local-1 $prot
  eos vid add gateway storage-local-2 $prot

  eos vid set map -$prot key:secretkey vuid:0 vgid:0
done
