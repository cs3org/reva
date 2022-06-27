#!/bin/bash

/usr/bin/freshclam
/usr/sbin/clamd
/usr/bin/c-icap -f /etc/c-icap/c-icap.conf -D

tail -f /var/log/c-icap/server.log
