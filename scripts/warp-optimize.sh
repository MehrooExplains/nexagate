#!/usr/bin/env bash
set -Eeuo pipefail

ACTIVE_IF=warp0
PROBE_IF=warp-probe
PROBE_NS=nexagate-warp-probe
PROBE_CONFIG=/etc/nexagate/warp-probe.conf
ACTIVE_CONFIG=/etc/wireguard/warp0.conf
STATE_DIR=/var/lib/nexagate/warp
LOCK_FILE=/run/nexagate-warp-optimize.lock

exec 9>"$LOCK_FILE"
flock -n 9 || exit 0

for command_name in ip wg curl awk sort sed grep; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "Missing required command: $command_name" >&2
    exit 1
  }
done
[[ -r $PROBE_CONFIG && -r $ACTIVE_CONFIG ]] || {
  echo "WARP probe configuration is missing" >&2
  exit 1
}

mkdir -p "$STATE_DIR"
WORK_DIR=$(mktemp -d /run/nexagate-warp.XXXXXX)
cleanup() {
  ip netns delete "$PROBE_NS" 2>/dev/null || true
  rm -rf -- "$WORK_DIR"
}
trap cleanup EXIT

ip netns delete "$PROBE_NS" 2>/dev/null || true
ip netns add "$PROBE_NS"
ip link add "$PROBE_IF" type wireguard
awk '!/^(Address|DNS|MTU|Table|PreUp|PostUp|PreDown|PostDown)[[:space:]]*=/' "$PROBE_CONFIG" >"$WORK_DIR/wg.conf"
wg setconf "$PROBE_IF" "$WORK_DIR/wg.conf"
PROBE_ADDRESS=$(awk -F= '/^Address[[:space:]]*=/{gsub(/[[:space:]]/,"",$2); sub(/,.*/,"",$2); print $2; exit}' "$PROBE_CONFIG")
[[ -n $PROBE_ADDRESS ]] || { echo "Probe Address is missing" >&2; exit 1; }
ip link set "$PROBE_IF" netns "$PROBE_NS"
ip -n "$PROBE_NS" link set lo up
ip -n "$PROBE_NS" address add "$PROBE_ADDRESS" dev "$PROBE_IF"
ip -n "$PROBE_NS" link set "$PROBE_IF" up
ip -n "$PROBE_NS" route replace default dev "$PROBE_IF"

PEER=$(ip netns exec "$PROBE_NS" wg show "$PROBE_IF" peers | head -n1)
[[ -n $PEER ]] || { echo "Probe WireGuard peer is missing" >&2; exit 1; }

CURRENT_ENDPOINT=$(wg show "$ACTIVE_IF" endpoints | awk 'NR==1{print $2}')
CURRENT_IP=${CURRENT_ENDPOINT%:*}
CURRENT_PORT=${CURRENT_ENDPOINT##*:}

CANDIDATES=()
for last_octet in {1..10}; do
  for port in 2408 500 4500 1701; do
    CANDIDATES+=("162.159.192.${last_octet}|${port}")
  done
done
if [[ $CURRENT_IP =~ ^[0-9.]+$ && $CURRENT_PORT =~ ^[0-9]+$ ]]; then
  CANDIDATES+=("${CURRENT_IP}|${CURRENT_PORT}")
fi

: >"$WORK_DIR/latency"
for candidate in "${CANDIDATES[@]}"; do
  ip_address=${candidate%|*}
  port=${candidate##*|}
  ip netns exec "$PROBE_NS" wg set "$PROBE_IF" peer "$PEER" endpoint "${ip_address}:${port}"
  sleep 0.15
  result=$(ip netns exec "$PROBE_NS" curl -4ksS --connect-timeout 2 --max-time 4 \
    -o /dev/null -w '%{time_total}' https://1.1.1.1/cdn-cgi/trace 2>/dev/null || true)
  [[ $result =~ ^[0-9]+([.][0-9]+)?$ ]] || continue
  latency_ms=$(awk -v value="$result" 'BEGIN { printf "%d", value * 1000 }')
  printf '%08d|%s|%s\n' "$latency_ms" "$ip_address" "$port" >>"$WORK_DIR/latency"
done

[[ -s $WORK_DIR/latency ]] || { echo "No WARP candidate passed the latency test" >&2; exit 1; }
sort -n "$WORK_DIR/latency" | head -n 4 >"$WORK_DIR/top"

: >"$WORK_DIR/scores"
while IFS='|' read -r latency_ms ip_address port; do
  latency_ms=$((10#$latency_ms))
  ip netns exec "$PROBE_NS" wg set "$PROBE_IF" peer "$PEER" endpoint "${ip_address}:${port}"
  sleep 0.2
  speed=$(ip netns exec "$PROBE_NS" curl -4ksS --connect-timeout 3 --max-time 12 \
    --resolve speed.cloudflare.com:443:104.16.249.249 \
    -o /dev/null -w '%{speed_download}' \
    'https://speed.cloudflare.com/__down?bytes=3000000' 2>/dev/null || true)
  [[ $speed =~ ^[0-9]+([.][0-9]+)?$ ]] || speed=0
  speed_bps=$(awk -v value="$speed" 'BEGIN { printf "%d", value }')
  score=$(awk -v bytes="$speed_bps" -v latency="$latency_ms" 'BEGIN { printf "%d", (bytes / 1000) - (latency * 8) }')
  printf '%d|%d|%d|%s|%s\n' "$score" "$speed_bps" "$latency_ms" "$ip_address" "$port" >>"$WORK_DIR/scores"
done <"$WORK_DIR/top"

BEST=$(sort -t'|' -k1,1nr "$WORK_DIR/scores" | head -n1)
IFS='|' read -r BEST_SCORE BEST_SPEED BEST_LATENCY BEST_IP BEST_PORT <<<"$BEST"
[[ -n ${BEST_IP:-} ]] || { echo "No WARP candidate passed the speed test" >&2; exit 1; }

CURRENT_SCORE=$(awk -F'|' -v ip="$CURRENT_IP" -v port="$CURRENT_PORT" '$4==ip && $5==port{print $1; exit}' "$WORK_DIR/scores")
SHOULD_SWITCH=1
if [[ $BEST_IP == "$CURRENT_IP" && $BEST_PORT == "$CURRENT_PORT" ]]; then
  SHOULD_SWITCH=0
elif [[ $CURRENT_SCORE =~ ^-?[0-9]+$ ]]; then
  threshold=$(awk -v score="$CURRENT_SCORE" 'BEGIN { printf "%d", score * 1.10 }')
  (( BEST_SCORE > threshold )) || SHOULD_SWITCH=0
fi

if (( SHOULD_SWITCH == 1 )); then
  ACTIVE_PEER=$(wg show "$ACTIVE_IF" peers | head -n1)
  [[ -n $ACTIVE_PEER ]] || { echo "Active WireGuard peer is missing" >&2; exit 1; }
  sed -Ei "s#^Endpoint[[:space:]]*=.*#Endpoint = ${BEST_IP}:${BEST_PORT}#" "$ACTIVE_CONFIG"
  wg set "$ACTIVE_IF" peer "$ACTIVE_PEER" endpoint "${BEST_IP}:${BEST_PORT}"
  ACTION=switched
else
  ACTION=kept
fi

STATUS_TMP=$(mktemp "$STATE_DIR/status.json.XXXXXX")
printf '{\n  "checked_at": "%s",\n  "action": "%s",\n  "endpoint": "%s:%s",\n  "latency_ms": %s,\n  "speed_bytes_per_second": %s\n}\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$ACTION" "$BEST_IP" "$BEST_PORT" "$BEST_LATENCY" "$BEST_SPEED" >"$STATUS_TMP"
chmod 0640 "$STATUS_TMP"
mv -f "$STATUS_TMP" "$STATE_DIR/status.json"
echo "WARP endpoint ${ACTION}: ${BEST_IP}:${BEST_PORT} (${BEST_LATENCY} ms, ${BEST_SPEED} B/s)"
