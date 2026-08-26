#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

REPOSITORY=MehrooExplains/nexagate
API_URL="https://api.github.com/repos/$REPOSITORY/releases/latest"
ASSET_NAME=nexagate-linux-amd64.tar.gz
STATE_DIR=/var/lib/nexagate
STATUS_FILE=$STATE_DIR/update-status.json
REQUEST_FILE=$STATE_DIR/update.request
LOCK_FILE=/run/lock/nexagate-update.lock
INSTALL_PATH=/usr/local/bin/nexagate
BACKUP_PATH=/usr/local/lib/nexagate/nexagate.previous
WORK_DIR=$(mktemp -d /tmp/nexagate-update.XXXXXX)

cleanup() {
  rm -rf -- "$WORK_DIR"
  rm -f -- "$REQUEST_FILE"
}
trap cleanup EXIT

write_status() {
  local state=$1 current=$2 latest=$3 message=$4 temporary
  temporary=$(mktemp "$STATE_DIR/.update-status.XXXXXX")
  jq -n \
    --arg state "$state" \
    --arg current "$current" \
    --arg latest "$latest" \
    --arg message "$message" \
    --arg updated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{state:$state,current_version:$current,latest_version:$latest,message:$message,updated_at:$updated_at}' >"$temporary"
  chmod 0644 "$temporary"
  mv -f -- "$temporary" "$STATUS_FILE"
}

fail() {
  local message=$1
  write_status failed "${CURRENT_VERSION:-unknown}" "${LATEST_VERSION:-}" "$message"
  printf '[NexaGate updater] %s\n' "$message" >&2
  exit 1
}

[[ $EUID -eq 0 ]] || { echo 'NexaGate updater must run as root.' >&2; exit 1; }
for command_name in awk curl date flock grep head install jq mktemp mv sha256sum sleep sort systemctl tail tar tr; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "Missing command: $command_name" >&2; exit 1; }
done
install -d -o nexagate-panel -g nexagate -m 0710 "$STATE_DIR"
exec 9>"$LOCK_FILE"
flock -n 9 || exit 0

CURRENT_VERSION=$("$INSTALL_PATH" version 2>/dev/null || printf unknown)
write_status checking "$CURRENT_VERSION" "" "Checking the latest GitHub release"

curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  --retry 3 --connect-timeout 15 --max-time 45 \
  -H 'Accept: application/vnd.github+json' \
  -H 'X-GitHub-Api-Version: 2022-11-28' \
  -H 'User-Agent: NexaGate-Updater' \
  --output "$WORK_DIR/release.json" "$API_URL" || fail "Could not read the latest GitHub release"

TAG=$(jq -r '.tag_name // empty' "$WORK_DIR/release.json")
[[ $TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]] || fail "The release tag is invalid"
LATEST_VERSION=${TAG#v}
if [[ $CURRENT_VERSION != unknown && $(printf '%s\n%s\n' "$CURRENT_VERSION" "$LATEST_VERSION" | sort -V | tail -n1) == "$CURRENT_VERSION" ]]; then
  write_status current "$CURRENT_VERSION" "$LATEST_VERSION" "NexaGate is already up to date"
  exit 0
fi

ASSET_URL=$(jq -r --arg name "$ASSET_NAME" '.assets[] | select(.name==$name) | .browser_download_url' "$WORK_DIR/release.json" | head -n1)
SUMS_URL=$(jq -r '.assets[] | select(.name=="SHA256SUMS") | .browser_download_url' "$WORK_DIR/release.json" | head -n1)
RELEASE_PREFIX="https://github.com/$REPOSITORY/releases/download/$TAG/"
[[ $ASSET_URL == "$RELEASE_PREFIX$ASSET_NAME" ]] || fail "The release archive URL does not match the selected tag"
[[ $SUMS_URL == "${RELEASE_PREFIX}SHA256SUMS" ]] || fail "The checksum URL does not match the selected tag"

write_status downloading "$CURRENT_VERSION" "$LATEST_VERSION" "Downloading and verifying the release"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 \
  --max-time 180 --output "$WORK_DIR/$ASSET_NAME" "$ASSET_URL" || fail "Could not download the release archive"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 \
  --max-time 45 --output "$WORK_DIR/SHA256SUMS" "$SUMS_URL" || fail "Could not download SHA256SUMS"

EXPECTED=$(awk -v name="$ASSET_NAME" '$2==name {print $1}' "$WORK_DIR/SHA256SUMS")
[[ $EXPECTED =~ ^[0-9a-fA-F]{64}$ ]] || fail "The release checksum is missing or invalid"
ACTUAL=$(sha256sum "$WORK_DIR/$ASSET_NAME" | awk '{print $1}')
[[ $ACTUAL == "$EXPECTED" ]] || fail "Release checksum verification failed"

if tar -tzf "$WORK_DIR/$ASSET_NAME" | grep -Ev '^(nexagate|VERSION)$' | grep -q .; then
  fail "The release archive contains unexpected paths"
fi
tar -xzf "$WORK_DIR/$ASSET_NAME" -C "$WORK_DIR" nexagate VERSION
[[ -x $WORK_DIR/nexagate && -f $WORK_DIR/VERSION ]] || fail "The release archive is incomplete"
[[ $(tr -d '\r\n' <"$WORK_DIR/VERSION") == "$LATEST_VERSION" ]] || fail "The release version does not match its tag"
[[ $("$WORK_DIR/nexagate" version) == "$LATEST_VERSION" ]] || fail "The release binary reports the wrong version"

write_status installing "$CURRENT_VERSION" "$LATEST_VERSION" "Installing the verified update"
install -m 0755 "$INSTALL_PATH" "$BACKUP_PATH"
install -o root -g root -m 0755 "$WORK_DIR/nexagate" "$INSTALL_PATH.new"
mv -f -- "$INSTALL_PATH.new" "$INSTALL_PATH"

if ! systemctl restart nexagate-panel.service; then
  install -o root -g root -m 0755 "$BACKUP_PATH" "$INSTALL_PATH"
  systemctl restart nexagate-panel.service || true
  fail "The updated panel failed to restart; the previous version was restored"
fi

LISTEN=$(jq -r '.listen // "127.0.0.1:9080"' /etc/nexagate/panel.json)
PANEL_OK=0
for _ in 1 2 3 4 5 6 7 8 9 10; do
  if systemctl is-active --quiet nexagate-panel.service && curl --fail --silent --max-time 2 "http://$LISTEN/healthz" >/dev/null; then
    PANEL_OK=1
    break
  fi
  sleep 1
done
if (( PANEL_OK == 0 )); then
  install -o root -g root -m 0755 "$BACKUP_PATH" "$INSTALL_PATH"
  systemctl restart nexagate-panel.service || true
  fail "Health verification failed; the previous version was restored"
fi

write_status success "$LATEST_VERSION" "$LATEST_VERSION" "Update installed successfully"
printf '[NexaGate updater] Updated %s -> %s\n' "$CURRENT_VERSION" "$LATEST_VERSION"
