#!/usr/bin/env bash

set -e

# kill running revad.
REVAD_PID=$(pgrep -f "revad" | tail -1) && kill -9 "${REVAD_PID}"
