#!/usr/bin/env bash
set -Eeuo pipefail

XRAY_UID=$(id -u nexagate-xray)
HYSTERIA_UID=$(id -u nexagate-hysteria)
DNS_UID=$(id -u nexagate-dns)

nft delete table inet nexagate 2>/dev/null || true
nft -f - <<EOF
table inet nexagate {
  chain output {
    type filter hook output priority -10; policy accept;

    oifname "lo" accept

    meta skuid $XRAY_UID oifname "warp0" accept
    meta skuid $XRAY_UID tcp sport { 443, 8444 } ct state established,related accept
    meta skuid $XRAY_UID reject with icmpx type admin-prohibited

    meta skuid $HYSTERIA_UID oifname "warp0" accept
    meta skuid $HYSTERIA_UID udp sport 443 ct state established,related accept
    meta skuid $HYSTERIA_UID reject with icmpx type admin-prohibited

    meta skuid $DNS_UID oifname "warp0" accept
    meta skuid $DNS_UID reject with icmpx type admin-prohibited
  }
}
EOF
