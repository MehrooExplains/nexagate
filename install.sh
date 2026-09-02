#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

VERSION=0.2.0
XRAY_VERSION=26.3.27
XRAY_SHA256=23cd9af937744d97776ee35ecad4972cf4b2109d1e0fe6be9930467608f7c8ae
HYSTERIA_VERSION=2.12.2
HYSTERIA_SHA256=6493dfffd55b5883f64c76c63880ecc32988f0c568c9ca9014907877b4d55f94
WGCF_VERSION=2.2.32
WGCF_SHA256=2ff97f2201972ce582a424455d50a3719a380eef0cd1f3144f7779348e122a2c
PSIPHON_COMMIT=50543d5e771ed7f2d6ccfedde9009fc2d3a799ff
PSIPHON_SHA256=47f8956f3f3cf9813d4cbee4665adc99b1f8ffa788c13dc5e03e824cc29217b0
CERTDUO_COMMIT=00ead2a80608c2907e4958650d5babab145ab169
CERTDUO_SHA256=4b5a8510acaba3c6b19d3bee5f801a3bf0311d2bc81eca14e8be4af5888bdf49
GO_VERSION=1.27.0
GO_SHA256=675c26c449cbb18fc24b74650de1eabbae6e16f64326fd85a283fb3b58280685
SOURCE_ARCHIVE_URL=https://codeload.github.com/MehrooExplains/nexagate/tar.gz/refs/heads/main
SOURCE_SCRIPT_URL=https://raw.githubusercontent.com/MehrooExplains/nexagate/main/install.sh

case ${1:-} in
  -h|--help)
    cat <<'HELP'
NexaGate interactive installer

Usage:
  bash <(curl -fsSL https://raw.githubusercontent.com/MehrooExplains/nexagate/main/install.sh)
  sudo ./install.sh

The installer checks prerequisites, obtains a domain or public-IP certificate
through CertDuo, installs verified runtime components, and enables NexaGate.
HELP
    exit 0
    ;;
  -v|--version)
    printf '%s\n' "$VERSION"
    exit 0
    ;;
  "") ;;
  *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
esac

say() { printf '\n[NexaGate] %s\n' "$*"; }
die() { printf '\n[NexaGate] ERROR: %s\n' "$*" >&2; exit 1; }

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

# A process-substitution command such as `bash <(curl ...)` gives Bash only this
# file. Download a stable temporary copy before sudo so the privileged process
# never depends on a potentially closed /dev/fd path.
if [[ ! -f $SCRIPT_DIR/go.mod || ! -d $SCRIPT_DIR/cmd/nexagate ]]; then
  if (( EUID != 0 )); then
    command -v sudo >/dev/null 2>&1 || die "Run as root, or install sudo and use an account allowed to run it."
    command -v curl >/dev/null 2>&1 || die "curl is required to download NexaGate."
    SELF_COPY_DIR=$(mktemp -d /tmp/nexagate-bootstrap-script.XXXXXX)
    curl --fail --location --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 \
      --output "$SELF_COPY_DIR/install.sh" "$SOURCE_SCRIPT_URL"
    chmod 0700 "$SELF_COPY_DIR/install.sh"
    trap '[[ $SELF_COPY_DIR == /tmp/nexagate-bootstrap-script.* ]] && rm -rf -- "$SELF_COPY_DIR"' EXIT
    sudo NEXAGATE_SELF_COPY_DIR="$SELF_COPY_DIR" bash "$SELF_COPY_DIR/install.sh" "$@"
    exit $?
  fi
fi

if (( EUID != 0 )); then
  command -v sudo >/dev/null 2>&1 || die "Run as root, or install sudo and use an account allowed to run it."
  exec sudo bash "$SCRIPT_DIR/install.sh" "$@"
fi

WORK_DIR=$(mktemp -d /tmp/nexagate-install.XXXXXX)
SOURCE_DIR=
REMOVE_BOOTSTRAP=0
cleanup() {
  if (( REMOVE_BOOTSTRAP == 1 )); then
    rm -f -- "${BOOTSTRAP_CONFIG:-}"
    systemctl reload nginx >/dev/null 2>&1 || true
  fi
  rm -rf -- "$WORK_DIR"
  if [[ -n ${SOURCE_DIR:-} && $SOURCE_DIR == /tmp/nexagate-source.* ]]; then
    rm -rf -- "$SOURCE_DIR"
  fi
  if [[ -n ${NEXAGATE_SELF_COPY_DIR:-} && $NEXAGATE_SELF_COPY_DIR == /tmp/nexagate-bootstrap-script.* ]]; then
    rm -rf -- "$NEXAGATE_SELF_COPY_DIR"
  fi
}
trap cleanup EXIT

[[ $(uname -s) == Linux ]] || die "Linux is required."
[[ $(uname -m) == x86_64 ]] || die "Version $VERSION supports amd64/x86_64 only because the official Psiphon binary is x86_64-only."
command -v systemctl >/dev/null 2>&1 || die "A systemd-based server is required."

if [[ ! -f $SCRIPT_DIR/go.mod || ! -d $SCRIPT_DIR/cmd/nexagate ]]; then
  say "Standalone installer detected; downloading the complete NexaGate source..."
  if ! command -v tar >/dev/null 2>&1 || ! command -v gzip >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl gzip tar
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y ca-certificates curl gzip tar
    elif command -v yum >/dev/null 2>&1; then
      yum install -y ca-certificates curl gzip tar
    else
      die "Install curl, tar, gzip, and CA certificates, then run the installer again."
    fi
  fi
  command -v curl >/dev/null 2>&1 || die "curl is required to download NexaGate."
  command -v tar >/dev/null 2>&1 || die "tar is required to unpack NexaGate."
  command -v gzip >/dev/null 2>&1 || die "gzip is required to unpack NexaGate."

  SOURCE_DIR=$(mktemp -d /tmp/nexagate-source.XXXXXX)
  curl --fail --location --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 \
    --output "$SOURCE_DIR/nexagate.tar.gz" "$SOURCE_ARCHIVE_URL"
  while IFS= read -r archive_path; do
    [[ $archive_path == nexagate-main || $archive_path == nexagate-main/* ]] || \
      die "The downloaded source archive contains an unexpected path."
    [[ $archive_path != /* && $archive_path != *'/../'* && $archive_path != ../* ]] || \
      die "The downloaded source archive contains an unsafe path."
  done < <(tar -tzf "$SOURCE_DIR/nexagate.tar.gz")
  tar -xzf "$SOURCE_DIR/nexagate.tar.gz" -C "$SOURCE_DIR"
  [[ -f $SOURCE_DIR/nexagate-main/go.mod && -d $SOURCE_DIR/nexagate-main/cmd/nexagate ]] || \
    die "The downloaded NexaGate source is incomplete."
  NEXAGATE_SELF_COPY_DIR='' bash "$SOURCE_DIR/nexagate-main/install.sh" "$@"
  exit $?
fi

[[ ! -e /etc/nexagate/panel.json ]] || die "NexaGate already appears installed. This installer will not overwrite its secrets or user database."

if command -v apt-get >/dev/null 2>&1; then
  PACKAGE_MANAGER=apt
elif command -v dnf >/dev/null 2>&1; then
  PACKAGE_MANAGER=dnf
elif command -v yum >/dev/null 2>&1; then
  PACKAGE_MANAGER=yum
else
  die "Supported package managers are apt, dnf, and yum."
fi

declare -a NEEDED_PACKAGES=()
need_command() {
  local command_name=$1 package_name=$2
  command -v "$command_name" >/dev/null 2>&1 || NEEDED_PACKAGES+=("$package_name")
}

say "Checking prerequisites before installation..."
if [[ $PACKAGE_MANAGER == apt ]]; then
  need_command curl curl
  need_command git git
  need_command unzip unzip
  need_command jq jq
  need_command openssl openssl
  need_command nginx nginx
  need_command wg wireguard-tools
  need_command nft nftables
  need_command qrencode qrencode
  need_command setfacl acl
  need_command ip iproute2
  need_command ss iproute2
  need_command flock util-linux
  need_command snap snapd
  need_command tar tar
  need_command timeout coreutils
  need_command getent libc-bin
  [[ -r /etc/ssl/certs/ca-certificates.crt ]] || NEEDED_PACKAGES+=(ca-certificates)
else
  need_command curl curl
  need_command git git
  need_command unzip unzip
  need_command jq jq
  need_command openssl openssl
  need_command nginx nginx
  need_command wg wireguard-tools
  need_command nft nftables
  need_command qrencode qrencode
  need_command setfacl acl
  need_command ip iproute
  need_command ss iproute
  need_command flock util-linux
  need_command snap snapd
  need_command tar tar
  need_command timeout coreutils
  need_command getent glibc-common
  [[ -d /etc/pki/ca-trust ]] || NEEDED_PACKAGES+=(ca-certificates)
fi

if (( ${#NEEDED_PACKAGES[@]} > 0 )); then
  mapfile -t NEEDED_PACKAGES < <(printf '%s\n' "${NEEDED_PACKAGES[@]}" | sort -u)
  say "Installing missing packages: ${NEEDED_PACKAGES[*]}"
  case "$PACKAGE_MANAGER" in
    apt)
      apt-get update
      DEBIAN_FRONTEND=noninteractive apt-get install -y "${NEEDED_PACKAGES[@]}"
      ;;
    dnf) dnf install -y "${NEEDED_PACKAGES[@]}" ;;
    yum) yum install -y "${NEEDED_PACKAGES[@]}" ;;
  esac
else
  say "All operating-system prerequisites are already installed."
fi

for command_name in curl git unzip jq openssl nginx wg nft qrencode setfacl ip ss flock snap tar timeout sha256sum getent; do
  command -v "$command_name" >/dev/null 2>&1 || die "Required command is still unavailable after package installation: $command_name"
done
nginx -V 2>&1 | grep -q -- '--with-http_v2_module' || \
  die "The installed Nginx build lacks HTTP/2 support required by the XHTTP TLS fallback."

if ip link show nwgcheck >/dev/null 2>&1; then
  die "Temporary preflight interface name 'nwgcheck' is already in use."
fi
ip link add nwgcheck type wireguard 2>/dev/null || die "The running kernel does not provide usable WireGuard support."
ip link delete nwgcheck
if ip netns list | grep -q '^nwgcheck '; then
  die "Temporary preflight network namespace name 'nwgcheck' is already in use."
fi
ip netns add nwgcheck 2>/dev/null || die "Network namespaces are unavailable; the isolated WARP optimizer cannot run."
ip netns delete nwgcheck
nft list ruleset >/dev/null || die "nftables is installed but cannot access the kernel ruleset."
ip link show warp0 >/dev/null 2>&1 && die "Network interface warp0 already exists."
if ip -4 rule show | grep -Eq 'lookup (51820|warp0)' || [[ -n $(ip -4 route show table 51820 2>/dev/null) ]]; then
  die "Policy-routing table 51820 is already in use."
fi

download_verified() {
  local url=$1 destination=$2 expected=$3
  curl --fail --location --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 15 --output "$destination" "$url"
  printf '%s  %s\n' "$expected" "$destination" | sha256sum --check --status || die "Checksum verification failed for $url"
}

tcp_port_busy() { ss -H -ltn | awk -v port=":$1" '$4 ~ port "$" {found=1} END{exit !found}'; }
udp_port_busy() { ss -H -lun | awk -v port=":$1" '$4 ~ port "$" {found=1} END{exit !found}'; }
for port in 443 2053 8443 8444; do
  tcp_port_busy "$port" && die "TCP port $port is already in use. Free it before installing NexaGate."
done
udp_port_busy 443 && die "UDP port 443 is already in use. Free it before installing NexaGate."
if tcp_port_busy 80 && ! ss -H -ltnp | awk '$4 ~ /:80$/ {print}' | grep -q nginx; then
  die "TCP port 80 is in use by a service other than Nginx. Free it before installing NexaGate."
fi

say "Certificate setup (powered by CertDuo)"
printf '1) Domain certificate\n2) Certificate for this server public IPv4\n'
read -r -p "Choose 1 or 2: " CERT_TYPE
[[ $CERT_TYPE == 1 || $CERT_TYPE == 2 ]] || die "Certificate choice must be 1 or 2."
read -r -p "Let's Encrypt account email: " CERT_EMAIL
[[ $CERT_EMAIL == *@*.* ]] || die "The email address is invalid."

if [[ $CERT_TYPE == 1 ]]; then
  read -r -p "Domain (example.com): " PUBLIC_HOST
  PUBLIC_HOST=${PUBLIC_HOST,,}
  [[ $PUBLIC_HOST =~ ^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$ ]] || die "The domain is invalid."
  SERVER_PUBLIC_IP=$(curl -4fsS --max-time 10 https://api.ipify.org) || die "Could not detect the server public IPv4 address."
  if ! getent ahostsv4 "$PUBLIC_HOST" | awk '{print $1}' | sort -u | grep -Fxq "$SERVER_PUBLIC_IP"; then
    die "The domain A record does not resolve to this server public IPv4 ($SERVER_PUBLIC_IP)."
  fi
else
  PUBLIC_HOST=$(curl -4fsS --max-time 10 https://api.ipify.org) || die "Could not detect the public IPv4 address."
  [[ $PUBLIC_HOST =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || die "The detected public IPv4 address is invalid."
  say "Detected public IPv4: $PUBLIC_HOST"
fi

ADMIN_PASSWORD=$(openssl rand -base64 24 | tr '/+' '_-')
[[ ${#ADMIN_PASSWORD} -ge 24 ]] || die "Could not generate the initial panel password."
# Keep installation non-interactive. The administrator can use the integrated
# scanner and change this default target later from Panel Settings.
REALITY_TARGET=www.microsoft.com

WEBROOT=/var/www/html
mkdir -p "$WEBROOT/.well-known/acme-challenge"
# CertDuo intentionally uses umask 077 for sensitive certificate material.
# Its short-lived HTTP-01 probe is therefore created as 0600 too. Grant only
# the Nginx worker an inherited read/traverse ACL on the public challenge
# directory so the probe and future renewals remain readable without making
# the rest of the installer workspace public.
NGINX_WORKER_USER=$(nginx -T 2>/dev/null | awk '$1 == "user" {gsub(/;/, "", $2); print $2; exit}')
if [[ -z $NGINX_WORKER_USER ]] || ! id "$NGINX_WORKER_USER" >/dev/null 2>&1; then
  for candidate in www-data nginx; do
    if id "$candidate" >/dev/null 2>&1; then NGINX_WORKER_USER=$candidate; break; fi
  done
fi
[[ -n $NGINX_WORKER_USER ]] || die "Could not determine the Nginx worker user."
setfacl -m "u:$NGINX_WORKER_USER:rx" "$WEBROOT" "$WEBROOT/.well-known"
setfacl -m "u:$NGINX_WORKER_USER:rwx,d:u:$NGINX_WORKER_USER:rx" "$WEBROOT/.well-known/acme-challenge"
BOOTSTRAP_CONFIG=/etc/nginx/conf.d/00-nexagate-bootstrap.conf
[[ ! -e $BOOTSTRAP_CONFIG && ! -e /etc/nginx/conf.d/nexagate.conf ]] || \
  die "An existing Nginx file uses a NexaGate-reserved configuration name."
install -d -m 0755 /etc/nginx/conf.d
sed -e "s/__HOST__/$PUBLIC_HOST/g" -e "s#__WEBROOT__#$WEBROOT#g" >"$BOOTSTRAP_CONFIG" <<'NGINX'
server {
    listen 80;
    listen [::]:80;
    server_name __HOST__;
    root __WEBROOT__;
    location ^~ /.well-known/acme-challenge/ { try_files $uri =404; }
    location / { return 404; }
}
NGINX
REMOVE_BOOTSTRAP=1
nginx -t || die "The temporary Nginx ACME configuration is invalid."
systemctl enable --now nginx
systemctl reload nginx

PREFLIGHT_TOKEN="nexagate-preflight-$RANDOM-$$"
printf '%s' "$PREFLIGHT_TOKEN" >"$WEBROOT/.well-known/acme-challenge/$PREFLIGHT_TOKEN"
PREFLIGHT_RESULT=$(curl -fsS --max-time 5 -H "Host: $PUBLIC_HOST" \
  "http://127.0.0.1/.well-known/acme-challenge/$PREFLIGHT_TOKEN" || true)
rm -f -- "$WEBROOT/.well-known/acme-challenge/$PREFLIGHT_TOKEN"
[[ $PREFLIGHT_RESULT == "$PREFLIGHT_TOKEN" ]] || \
  die "Nginx cannot read the HTTP-01 webroot through its worker account ($NGINX_WORKER_USER)."

say "Downloading the pinned and checksum-verified CertDuo release..."
download_verified \
  "https://raw.githubusercontent.com/MehrooExplains/certduo/$CERTDUO_COMMIT/certduo.sh" \
  "$WORK_DIR/certduo.sh" "$CERTDUO_SHA256"
chmod 0700 "$WORK_DIR/certduo.sh"
if [[ $CERT_TYPE == 1 ]]; then
  printf '1\n%s\n%s\n%s\nn\n' "$CERT_EMAIL" "$WEBROOT" "$PUBLIC_HOST" | "$WORK_DIR/certduo.sh"
else
  printf '2\n%s\n%s\n' "$CERT_EMAIL" "$WEBROOT" | "$WORK_DIR/certduo.sh"
fi
CERT_NAME=$PUBLIC_HOST
CERT_FILE=/etc/letsencrypt/live/$CERT_NAME/fullchain.pem
KEY_FILE=/etc/letsencrypt/live/$CERT_NAME/privkey.pem
[[ -s $CERT_FILE && -s $KEY_FILE ]] || die "CertDuo finished without the expected certificate files."

say "Downloading pinned runtime components and verifying every checksum..."
download_verified "https://github.com/XTLS/Xray-core/releases/download/v$XRAY_VERSION/Xray-linux-64.zip" "$WORK_DIR/xray.zip" "$XRAY_SHA256"
download_verified "https://github.com/HyNetworks/hysteria/releases/download/app/v$HYSTERIA_VERSION/hysteria-linux-amd64" "$WORK_DIR/hysteria" "$HYSTERIA_SHA256"
download_verified "https://github.com/ViRb3/wgcf/releases/download/v$WGCF_VERSION/wgcf_${WGCF_VERSION}_linux_amd64" "$WORK_DIR/wgcf" "$WGCF_SHA256"
download_verified "https://raw.githubusercontent.com/Psiphon-Labs/psiphon-tunnel-core-binaries/$PSIPHON_COMMIT/linux/psiphon-tunnel-core-x86_64" "$WORK_DIR/psiphon" "$PSIPHON_SHA256"

install -d -m 0755 /usr/local/lib/nexagate
unzip -jo "$WORK_DIR/xray.zip" xray -d /usr/local/lib/nexagate >/dev/null
install -m 0755 "$WORK_DIR/hysteria" /usr/local/lib/nexagate/hysteria
install -m 0755 "$WORK_DIR/wgcf" /usr/local/lib/nexagate/wgcf
install -m 0755 "$WORK_DIR/psiphon" /usr/local/lib/nexagate/psiphon-tunnel-core
install -m 0755 "$WORK_DIR/certduo.sh" /usr/local/lib/nexagate/certduo.sh
timeout 10s /usr/local/lib/nexagate/xray tls ping "$REALITY_TARGET" >/dev/null 2>&1 || \
  die "The selected REALITY camouflage target did not pass Xray's TLS handshake test."

GO_BIN=
if command -v go >/dev/null 2>&1; then
  GO_CURRENT=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
  if [[ $(printf '%s\n' 1.22 "$GO_CURRENT" | sort -V | head -n1) == 1.22 ]]; then
    GO_BIN=$(command -v go)
  fi
fi
if [[ -z $GO_BIN ]]; then
  say "A suitable Go compiler was not found; downloading a temporary verified Go $GO_VERSION toolchain..."
  download_verified "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" "$WORK_DIR/go.tar.gz" "$GO_SHA256"
  tar -C "$WORK_DIR" -xzf "$WORK_DIR/go.tar.gz"
  GO_BIN=$WORK_DIR/go/bin/go
fi

say "Building and testing the NexaGate panel..."
(cd "$SCRIPT_DIR" && GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" test ./...)
(cd "$SCRIPT_DIR" && GOTOOLCHAIN=local CGO_ENABLED=0 "$GO_BIN" build -buildvcs=false -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$WORK_DIR/nexagate" ./cmd/nexagate)
install -m 0755 "$WORK_DIR/nexagate" /usr/local/bin/nexagate

getent group nexagate >/dev/null 2>&1 || groupadd --system nexagate
for account in panel xray hysteria psiphon dns; do
  user_name=nexagate-$account
  getent group "$user_name" >/dev/null 2>&1 || groupadd --system "$user_name"
  id "$user_name" >/dev/null 2>&1 || useradd --system --gid "$user_name" --groups nexagate --no-create-home --shell /usr/sbin/nologin "$user_name"
done
install -d -o nexagate-panel -g nexagate -m 0710 /etc/nexagate /etc/nexagate/generated /var/lib/nexagate
install -d -o nexagate-psiphon -g nexagate-psiphon -m 0750 /var/lib/nexagate/psiphon
install -d -o nexagate-hysteria -g nexagate-hysteria -m 0750 /var/lib/nexagate/hysteria
install -d -o root -g root -m 0750 /var/lib/nexagate/warp

say "Registering two independent WARP profiles (active and non-disruptive probe)..."
mkdir -p "$WORK_DIR/warp-active" "$WORK_DIR/warp-probe"
(cd "$WORK_DIR/warp-active" && /usr/local/lib/nexagate/wgcf register --accept-tos && /usr/local/lib/nexagate/wgcf generate --profile)
(cd "$WORK_DIR/warp-probe" && /usr/local/lib/nexagate/wgcf register --accept-tos && /usr/local/lib/nexagate/wgcf generate --profile)
ACTIVE_PROFILE=$WORK_DIR/warp-active/wgcf-profile.conf
PROBE_PROFILE=$WORK_DIR/warp-probe/wgcf-profile.conf
[[ -s $ACTIVE_PROFILE && -s $PROBE_PROFILE ]] || die "wgcf did not generate both WARP profiles."
WARP_ADDRESS=$(awk -F= '/^Address[[:space:]]*=/{gsub(/[[:space:]]/,"",$2); sub(/,.*/,"",$2); print $2; exit}' "$ACTIVE_PROFILE")
[[ $WARP_ADDRESS == */* ]] || die "The WARP profile does not contain an IPv4 Address."
awk -v address="$WARP_ADDRESS" '
  /^DNS[[:space:]]*=/ { next }
  /^MTU[[:space:]]*=/ { print; next }
  /^\[Peer\]/ && !inserted {
    print "Table = off"
    print "PostUp = /usr/local/lib/nexagate/warp-route.sh up %i " address
    print "PostDown = /usr/local/lib/nexagate/warp-route.sh down %i " address
    inserted=1
  }
  { print }
' "$ACTIVE_PROFILE" >"$WORK_DIR/warp0.conf"
install -d -m 0700 /etc/wireguard
install -m 0600 "$WORK_DIR/warp0.conf" /etc/wireguard/warp0.conf
install -m 0600 "$PROBE_PROFILE" /etc/nexagate/warp-probe.conf

install -m 0755 "$SCRIPT_DIR/scripts/apply-config.sh" /usr/local/lib/nexagate/apply-config.sh
install -m 0755 "$SCRIPT_DIR/scripts/apply-firewall.sh" /usr/local/lib/nexagate/apply-firewall.sh
install -m 0755 "$SCRIPT_DIR/scripts/psiphon-health.sh" /usr/local/lib/nexagate/psiphon-health.sh
install -m 0755 "$SCRIPT_DIR/scripts/warp-optimize.sh" /usr/local/lib/nexagate/warp-optimize.sh
install -m 0755 "$SCRIPT_DIR/scripts/warp-route.sh" /usr/local/lib/nexagate/warp-route.sh
install -m 0755 "$SCRIPT_DIR/scripts/update.sh" /usr/local/lib/nexagate/update.sh

REALITY_OUTPUT=$(/usr/local/lib/nexagate/xray x25519)
REALITY_PRIVATE=$(awk -F': ' 'NR==1{print $2}' <<<"$REALITY_OUTPUT")
REALITY_PUBLIC=$(awk -F': ' 'NR==2{print $2}' <<<"$REALITY_OUTPUT")
[[ -n $REALITY_PRIVATE && -n $REALITY_PUBLIC ]] || die "Could not generate the REALITY X25519 key pair."
printf '%s\n' "$ADMIN_PASSWORD" >"$WORK_DIR/admin-password"
printf '%s\n' "$REALITY_PRIVATE" >"$WORK_DIR/reality-private"
unset REALITY_PRIVATE REALITY_OUTPUT

say "Initializing the panel and generated service configurations..."
/usr/local/bin/nexagate init \
  --config /etc/nexagate/panel.json \
  --state /var/lib/nexagate/users.json \
  --generated-dir /etc/nexagate/generated \
  --host "$PUBLIC_HOST" \
  --cert-name "$CERT_NAME" \
  --webroot "$WEBROOT" \
  --admin-password-file "$WORK_DIR/admin-password" \
  --reality-private-key-file "$WORK_DIR/reality-private" \
  --reality-public-key "$REALITY_PUBLIC" \
  --reality-target "$REALITY_TARGET"
chown -R nexagate-panel:nexagate-panel /etc/nexagate /var/lib/nexagate
chown nexagate-panel:nexagate /etc/nexagate /etc/nexagate/generated /var/lib/nexagate
chmod 0710 /etc/nexagate /etc/nexagate/generated /var/lib/nexagate
chown -R nexagate-psiphon:nexagate-psiphon /var/lib/nexagate/psiphon
chown -R nexagate-hysteria:nexagate-hysteria /var/lib/nexagate/hysteria
chown -R root:root /var/lib/nexagate/warp
chown root:root /etc/nexagate/warp-probe.conf
chmod 0600 /etc/nexagate/panel.json
chmod 0600 /var/lib/nexagate/users.json
chmod 0600 /etc/nexagate/generated/*
chmod 0600 /etc/nexagate/warp-probe.conf

setfacl -m u:nexagate-hysteria:x /etc/letsencrypt /etc/letsencrypt/live /etc/letsencrypt/archive
setfacl -R -m u:nexagate-hysteria:rX "/etc/letsencrypt/live/$CERT_NAME"
ARCHIVE_DIR=$(dirname "$(readlink -f "$CERT_FILE")")
setfacl -R -m u:nexagate-hysteria:rX "$ARCHIVE_DIR"
setfacl -m d:u:nexagate-hysteria:rX "$ARCHIVE_DIR"

rm -f "$BOOTSTRAP_CONFIG"
REMOVE_BOOTSTRAP=0
ln -sfn /etc/nexagate/generated/nginx.conf /etc/nginx/conf.d/nexagate.conf
nginx -t || die "Generated Nginx configuration validation failed."

install -m 0644 "$SCRIPT_DIR"/systemd/*.service "$SCRIPT_DIR"/systemd/*.path "$SCRIPT_DIR"/systemd/*.timer /etc/systemd/system/
install -d -m 0755 /etc/letsencrypt/renewal-hooks/deploy
install -m 0755 /dev/stdin /etc/letsencrypt/renewal-hooks/deploy/nexagate.sh <<'HOOK'
#!/usr/bin/env bash
set -eu
systemctl reload nginx
systemctl restart nexagate-hysteria.service
HOOK

systemctl daemon-reload
systemctl enable --now wg-quick@warp0.service
/usr/local/lib/nexagate/apply-config.sh
systemctl enable --now nexagate-firewall.service
systemctl enable --now nexagate-psiphon.service nexagate-dns.service
systemctl enable --now nexagate-xray.service nexagate-hysteria.service nexagate-panel.service
systemctl enable --now nexagate-apply.path nexagate-update.path nexagate-warp-optimize.timer nexagate-psiphon-health.timer
systemctl reload nginx

if systemctl is-active --quiet ufw; then
  for rule in 80/tcp 443/tcp 443/udp 2053/tcp 8443/tcp 8444/tcp; do ufw allow "$rule"; done
elif systemctl is-active --quiet firewalld; then
  for rule in 80/tcp 443/tcp 443/udp 2053/tcp 8443/tcp 8444/tcp; do firewall-cmd --permanent --add-port="$rule"; done
  firewall-cmd --reload
fi

say "Installation completed successfully."
INITIAL_CREDENTIALS=/root/nexagate-initial-credentials.txt
install -m 0600 /dev/null "$INITIAL_CREDENTIALS"
{
  printf 'Panel URL: https://%s:8443/\n' "$PUBLIC_HOST"
  printf 'Initial password: %s\n' "$ADMIN_PASSWORD"
} >"$INITIAL_CREDENTIALS"
printf '\nPanel URL: https://%s:8443/\n' "$PUBLIC_HOST"
printf 'Initial administrator password: %s\n' "$ADMIN_PASSWORD"
printf 'A root-only copy is stored at %s. Delete it after changing the password in Panel Settings.\n' "$INITIAL_CREDENTIALS"
unset ADMIN_PASSWORD
printf 'Routing: TCP -> Psiphon, UDP/DNS -> WARP (fail closed)\n'
printf 'Run diagnostics: sudo nexagate doctor\n'
printf 'Important: also open TCP 80,443,2053,8443,8444 and UDP 443 in your cloud firewall/security group.\n'
