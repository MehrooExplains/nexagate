#!/usr/bin/env bash
set -Eeuo pipefail

XRAY=/usr/local/lib/nexagate/xray
HYSTERIA=/usr/local/lib/nexagate/hysteria
GENERATED=/etc/nexagate/generated
HY_TEST=$(mktemp /run/nexagate-hysteria-check.XXXXXX.yaml)
cleanup() { rm -f -- "$HY_TEST"; }
trap cleanup EXIT

"$XRAY" run -test -config "$GENERATED/xray.json"
awk 'NR==1{$0="listen: :0"} /^  listen: 127[.]0[.]0[.]1:[0-9]+$/{$0="  listen: 127.0.0.1:0"} {print}' \
  "$GENERATED/hysteria.yaml" >"$HY_TEST"
set +e
timeout 2s "$HYSTERIA" server --config "$HY_TEST" --disable-update-check >/dev/null 2>&1
HY_STATUS=$?
set -e
[[ $HY_STATUS -eq 124 ]] || { echo "Hysteria configuration validation failed" >&2; exit 1; }
nginx -t

chown nexagate-panel:nexagate-xray "$GENERATED/xray.json"
chown nexagate-panel:nexagate-hysteria "$GENERATED/hysteria.yaml"
chown nexagate-panel:nexagate-psiphon "$GENERATED/psiphon.json"
chown nexagate-panel:nexagate-panel "$GENERATED/nginx.conf"
chmod 0640 "$GENERATED/xray.json" "$GENERATED/hysteria.yaml" "$GENERATED/psiphon.json"
chmod 0600 "$GENERATED/nginx.conf"

systemctl try-restart nexagate-psiphon.service
systemctl try-restart nexagate-xray.service
systemctl try-restart nexagate-hysteria.service
systemctl reload nginx
