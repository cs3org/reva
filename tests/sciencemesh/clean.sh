#!/usr/bin/env bash

set -e

running=$(docker ps -q)
([ -z "$running" ] && echo "no running containers!") || docker kill $running
existing=$(docker ps -qa)
([ -z "$existing" ] && echo "no existing containers!") || docker rm $existing
docker network remove testnet || true
docker network create testnet
