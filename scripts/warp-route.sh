#!/usr/bin/env bash
set -Eeuo pipefail

ACTION=${1:?up or down is required}
INTERFACE=${2:?interface is required}
ADDRESS=${3:?IPv4 address is required}
SOURCE_IP=${ADDRESS%%/*}
TABLE=51820
PRIORITY=100

case "$ACTION" in
  up)
    ip -4 route replace default dev "$INTERFACE" table "$TABLE"
    if ! ip -4 rule show | grep -Fq "from $SOURCE_IP lookup $TABLE"; then
      ip -4 rule add priority "$PRIORITY" from "$SOURCE_IP/32" table "$TABLE"
    fi
    ;;
  down)
    ip -4 rule delete priority "$PRIORITY" from "$SOURCE_IP/32" table "$TABLE" 2>/dev/null || true
    ip -4 route flush table "$TABLE" 2>/dev/null || true
    ;;
  *)
    echo "Usage: $0 {up|down} INTERFACE ADDRESS" >&2
    exit 2
    ;;
esac
