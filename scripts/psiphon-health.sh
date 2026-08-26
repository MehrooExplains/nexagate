#!/usr/bin/env bash
set -Eeuo pipefail

for test_url in https://checkip.amazonaws.com/ https://www.cloudflare.com/cdn-cgi/trace; do
  if curl --silent --show-error --fail --max-time 15 \
    --socks5-hostname 127.0.0.1:1080 "$test_url" >/dev/null; then
    exit 0
  fi
done

systemctl restart nexagate-psiphon.service
exit 1
