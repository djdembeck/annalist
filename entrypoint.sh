#!/bin/sh
set -e

mkdir -p /app/data/clones
chown -R annalist:annalist /app/data 2>/dev/null || true

exec su-exec annalist:annalist "$@"
