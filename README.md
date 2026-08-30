# NexaGate

**Multi-inbound Linux gateway with TCP over Psiphon and UDP/DNS over WARP**

<p align="center">
  <strong><img src="https://commons.wikimedia.org/wiki/Special:Redirect/file/Flag_of_the_United_Kingdom.svg?width=48" width="28" alt="United Kingdom flag"> English</strong>
  &nbsp;|&nbsp;
  <a href="README.fa.md"><img src="https://commons.wikimedia.org/wiki/Special:Redirect/file/State_flag_of_the_Imperial_State_of_Iran_(with_standardized_lion_and_sun).svg?width=48" width="28" alt="Iranian Lion and Sun flag"> فارسی</a>
</p>

[![Checks](https://github.com/MehrooExplains/nexagate/actions/workflows/checks.yml/badge.svg)](https://github.com/MehrooExplains/nexagate/actions/workflows/checks.yml)

NexaGate is an early Linux server project that combines five censorship-resistant inbounds with two deliberately separated egress paths:

- inner **TCP** traffic exits through the **Psiphon** network;
- inner **UDP** traffic and tunnel DNS exit through **Cloudflare WARP**;
- a fail-closed firewall prevents Xray, Hysteria, and the DNS relay from silently falling back to the server's normal Internet route.

> Project status: `0.2.0`. Test it on a fresh, recoverable server before using it in production. No protocol or server can guarantee access under every filtering policy.

## Architecture

```text
Clients
  ├─ UDP/443  ─ Hysteria2
  ├─ TCP/443  ─ VLESS + XHTTP + REALITY
  ├─ TCP/8444 ─ VLESS + RAW + REALITY + Vision
  └─ TCP/2053 ─ Nginx TLS/HTTP2
                    ├─ VLESS + XHTTP + TLS
                    └─ VLESS + WebSocket + TLS
                         │
                    TCP / UDP split
                     /           \
          TCP → Psiphon       UDP → WARP (warp0)
                    DNS → local relay → WARP

Management: TCP/8443 → Nginx HTTPS → panel on 127.0.0.1:9080
```

### Inbounds and ports

| Public port | Protocol | Purpose |
|---|---|---|
| UDP `443` | Hysteria2 + Salamander | Fast UDP-based primary profile |
| TCP `443` | VLESS + XHTTP + REALITY | Resistant HTTPS-like profile |
| TCP `8444` | VLESS + RAW + REALITY + Vision | Compatible REALITY profile |
| TCP `2053` | VLESS + XHTTP + TLS over HTTP/2 | Real-HTTPS fallback when REALITY or UDP is disrupted |
| TCP `2053` | VLESS + WebSocket + TLS | Legacy emergency fallback on a separate secret path |
| TCP `8443` | HTTPS | Administrator panel |
| TCP `80` | HTTP-01 only | Let's Encrypt issuance and renewal |

TCP and UDP may use port `443` at the same time because they are different transports.

## Main features

- Separate Persian (RTL) and English (LTR) interface modes with a persistent language choice and responsive left navigation sidebar; labels are never mixed on the same screen
- Dedicated overview, users, inbounds, outbounds, routing, and panel-settings pages
- Live overview cards for CPU, RAM, swap, storage, network speed/totals, sockets, load average, uptime, and panel resource use
- Self-hosted Inter and Vazirmatn UI typography, with JetBrains Mono tabular numerals for stable live metrics and no font-CDN dependency
- Self-contained Lucide SVG icons with consistent 20 px navigation sizing and no icon-CDN dependency
- Server IP addresses blurred by default and revealed only with the eye control
- One-click panel updates with release checksum verification, post-update health check, and automatic rollback
- Search, add, disable, expire, and delete users from a focused account-management screen
- Five connection links and QR codes per user, including the real-TLS XHTTP fallback
- Automatic, backward-compatible configuration migration when a binary-only one-click update introduces new generated settings
- Automatic Psiphon server selection, or an optional fixed two-letter region such as `DE`
- Automatic WARP endpoint selection using latency and throughput probes
- A separate WARP probe interface/network namespace, so benchmarking does not interrupt active users
- DNS relay whose upstream socket is locked to `warp0`
- Owner-based nftables kill switch for Xray, Hysteria, and DNS
- Local-only Psiphon SOCKS proxy and panel backend
- Hardened, unprivileged systemd services
- Atomic configuration writes and validation before service reloads
- Automatic health checks for Psiphon
- Domain and public-IP certificates through [CertDuo](https://github.com/MehrooExplains/certduo)
- Automatic prerequisite detection: only missing packages are installed before setup continues
- SHA-256 verification for all downloaded pinned binaries and scripts

### Pinned components in 0.2.0

| Component | Pinned release |
|---|---|
| Xray-core | `26.3.27` |
| Hysteria2 | `2.12.2` |
| wgcf | `2.2.32` |
| Psiphon tunnel core | repository commit `50543d5e771e` |
| CertDuo | repository commit `550b5b0b5540` |
| Temporary Go toolchain, when required | `1.27.0` |

The complete commit IDs and SHA-256 values are kept in `install.sh`; a checksum mismatch stops installation.

## Supported systems

The initial release supports:

- a fresh systemd-based Linux server;
- `amd64` / `x86_64` architecture;
- `apt`, `dnf`, or `yum`;
- a public IPv4 address;
- root or `sudo` access.

The architecture restriction exists because Psiphon's official Linux client binary used by this release is x86_64-only. Debian/Ubuntu receives the most testing; RHEL-like package availability can vary by repository.

### Required open ports

Open these in both the server firewall and the hosting provider's security group:

```text
TCP: 80, 443, 2053, 8443, 8444
UDP: 443
```

For a domain certificate, create an `A` record pointing to this server before installation. Port `80` must be publicly reachable for HTTP-01 validation. Do not place an HTTP proxy/CDN in front of the initial certificate request unless it passes the challenge unchanged.

## Installation

Run the interactive installer with one command from an account that has root or `sudo` access:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MehrooExplains/nexagate/main/install.sh)
```

The standalone script automatically elevates with `sudo` when necessary, downloads the complete project from this repository, checks prerequisites, installs only missing packages, and then continues with the interactive setup. To audit it first, open [install.sh](install.sh); cloning the repository and running `sudo ./install.sh` remains supported.

The installer asks for:

1. certificate type: domain or this server's public IP;
2. Let's Encrypt account email;
3. domain name when option 1 is selected;
4. a panel administrator password;
5. an optional REALITY camouflage target.

It then performs this sequence:

1. verifies Linux, systemd, x86_64, free ports, and available package manager;
2. checks every prerequisite and installs only missing packages;
3. starts a minimal Nginx HTTP-01 webroot;
4. downloads the pinned CertDuo script and verifies its checksum;
5. uses CertDuo to issue either the requested domain certificate or a short-lived certificate for the detected public IPv4;
6. downloads and verifies Xray, Hysteria2, wgcf, Psiphon, and—only when necessary—a temporary Go toolchain;
7. builds and tests NexaGate locally;
8. creates two WARP registrations: one active profile and one isolated benchmark profile;
9. generates REALITY keys, secrets, service configs, firewall rules, and systemd units;
10. validates Xray, Hysteria, and Nginx configurations, then enables the services.

The installer deliberately refuses to overwrite an existing `/etc/nexagate/panel.json` installation.

## CertDuo integration

NexaGate does not reimplement certificate issuance. It pins a known CertDuo commit, verifies the script with SHA-256, and supplies the answers selected in the NexaGate installer.

### Domain mode

CertDuo requests a normal Let's Encrypt domain certificate with HTTP-01 webroot validation. The resulting files are read from:

```text
/etc/letsencrypt/live/DOMAIN/fullchain.pem
/etc/letsencrypt/live/DOMAIN/privkey.pem
```

### Public-IP mode

CertDuo detects the server's current public IPv4 and requests Let's Encrypt's short-lived IP certificate profile. These certificates have a much shorter lifetime than ordinary domain certificates, so automatic renewal must remain enabled. The installer adds a deployment hook that reloads Nginx and restarts Hysteria after renewal.

## Real-TLS XHTTP fallback

Every user receives a fifth profile: **VLESS + XHTTP + TLS**. It uses the real CertDuo certificate and HTTP/2 on public TCP `2053`. Nginx terminates TLS and forwards only the randomly generated XHTTP path over local h2c to Xray on `127.0.0.1:10002`; the existing WebSocket fallback keeps a different secret path on the same public port.

Recommended connection order:

1. XHTTP + REALITY on TCP `443`;
2. XHTTP + real TLS on TCP `2053` when REALITY is impaired;
3. Hysteria2 on UDP `443` when the current network has reliable UDP;
4. RAW REALITY or WebSocket TLS for client compatibility and emergency use.

The XHTTP TLS link pins ALPN to HTTP/2 and uses XHTTP `auto` mode. Inner TCP still exits through Psiphon and inner UDP/DNS still exits through WARP. A server-side protocol cannot detect the user's Iranian ISP before the connection arrives, so profile selection remains explicit in the client.

## Psiphon behavior

Psiphon provides a local SOCKS5 proxy on `127.0.0.1:1080`. Xray and Hysteria send only TCP sessions to it.

- Leave the region field blank in the panel for Psiphon's automatic selection and reconnection behavior.
- Enter a two-letter ISO country code, for example `DE`, `NL`, or `CA`, to request a fixed egress region.

The panel rewrites the Psiphon configuration and triggers a controlled service restart. A five-minute health timer tests the SOCKS egress and restarts Psiphon if the test fails. A fixed region is a preference exposed by Psiphon; availability is not guaranteed.

## WARP behavior

WARP is used for inner UDP and DNS, without country selection. Every 30 minutes the optimizer:

1. creates a temporary network namespace;
2. loads the independent probe WARP profile;
3. measures candidate Cloudflare endpoint/port latency;
4. measures throughput for the best latency candidates;
5. keeps the current endpoint when it remains competitive, avoiding unnecessary churn;
6. updates the active endpoint only when a materially better result is found.

The latest result is stored at `/var/lib/nexagate/warp/status.json`. The probe namespace is removed after every run. Using WARP is subject to Cloudflare's applicable terms and service availability.

## Fail-closed routing

Application routing and the operating-system firewall both enforce the split:

- Xray/Hysteria TCP → local Psiphon SOCKS;
- Xray/Hysteria UDP → interface `warp0`;
- DNS upstream → interface `warp0`;
- unmatched traffic → reject/blackhole.

The nftables owner rules allow public inbound replies, loopback access to Psiphon/DNS, and WARP-bound output. Other output owned by the Xray, Hysteria, or DNS service accounts is rejected. Psiphon itself is intentionally allowed to reach its network directly because it is the TCP egress transport.

## Administration

Open the panel:

```text
https://YOUR_DOMAIN_OR_IP:8443/
```

### Live overview

The overview refreshes Linux host metrics every two seconds without installing a separate monitoring daemon or a browser chart library. CPU, memory, swap, root filesystem, socket, process, and uptime data come from Linux `/proc` and filesystem counters. Upload/download rates and cumulative totals refer to the server's default-route interface; totals are since that interface was started and are operational indicators, not provider billing records.

The panel lists the non-loopback IPv4 and IPv6 addresses on the server's default-route interface. They are blurred by default; select the eye icon to reveal them and select it again to hide them. This is visual privacy for screen sharing, not an access-control boundary; only authenticated administrators can open the metrics API.

Persian and English are independent interface modes. The language button stores the administrator's choice in a same-site cookie, switches the document between RTL and LTR, and keeps the administrator on the current page. Natural-language labels from both languages are never combined in one view; protocol and product identifiers such as TCP, Hysteria2, Psiphon, and WARP remain unchanged.

### One-click panel updates

Select **Check & update** in Overview to request an update. The web process only writes a narrowly scoped request file; a separate root-owned systemd service performs the privileged work. It:

1. reads the latest release from the fixed `MehrooExplains/nexagate` repository;
2. downloads the Linux amd64 archive and `SHA256SUMS` from the same tagged release;
3. validates the tag, download URLs, archive paths, checksum, embedded version, and binary-reported version;
4. keeps the previous panel executable, installs the new one atomically, and restarts the panel;
5. checks `/healthz` and automatically restores the previous executable if startup or health verification fails.

The button updates the NexaGate executable. On first startup, a new binary safely fills missing configuration defaults, writes them atomically, regenerates service configuration, and lets the existing validation path apply it. A future release that explicitly changes operating-system packages or systemd/network policy may still require its documented migration procedure. SHA-256 protects release integrity in transit and detects mismatches; trust still depends on the GitHub repository and release account. Release builds are created by `.github/workflows/release.yml`; development builds intentionally disable the update button.

Useful checks:

```bash
sudo nexagate doctor
sudo systemctl status nexagate-panel nexagate-xray nexagate-hysteria nexagate-psiphon
sudo journalctl -u nexagate-psiphon -u nexagate-hysteria -n 100 --no-pager
sudo systemctl start nexagate-warp-optimize.service
cat /var/lib/nexagate/warp/status.json
```

Important paths:

| Path | Description |
|---|---|
| `/etc/nexagate/panel.json` | Panel and routing configuration; contains secrets |
| `/var/lib/nexagate/users.json` | User database and generated credentials |
| `/etc/nexagate/generated/` | Generated Xray, Hysteria, Psiphon, and Nginx configs |
| `/etc/wireguard/warp0.conf` | Active WARP profile |
| `/etc/nexagate/warp-probe.conf` | Independent optimizer profile |
| `/var/lib/nexagate/warp/status.json` | Latest WARP benchmark result |
| `/var/lib/nexagate/update-status.json` | Last panel update state shown in Overview |

Back up the first five sensitive paths securely. Never publish them or paste them into an issue.

## Troubleshooting

- Run `sudo nexagate doctor` first.
- Confirm cloud firewall rules as well as the local firewall.
- For a domain, confirm its `A` record resolves directly to the server.
- Confirm TCP `80` remains reachable for renewal.
- Check `systemctl status wg-quick@warp0` if UDP or DNS fails.
- Check `127.0.0.1:1080` and the Psiphon journal if TCP fails.
- If only one profile fails, inspect its corresponding Xray/Hysteria service rather than changing both egress paths.

## Security notes and limitations

- The panel backend listens only on loopback and is published through HTTPS on `8443`.
- Administrator passwords use PBKDF2-HMAC-SHA-256 with a random salt; sessions are signed, secure, HTTP-only, and SameSite strict.
- The panel applies login throttling, CSRF validation, security headers, and atomic state writes.
- Runtime services use separate unprivileged accounts and restrictive systemd sandboxes.
- Configuration links contain credentials. Anyone who receives one can use that user until it is disabled, deleted, or expires.
- Traffic inside Psiphon and WARP ultimately depends on third-party networks and their policies.
- WARP is not used to choose a country. Psiphon fixed-region availability may change.
- This project cannot promise invisibility, unrestricted access, or compatibility with every ISP/DPI system.

## Development

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
bash -n install.sh scripts/*.sh
shellcheck install.sh scripts/*.sh
```

The test suite can additionally validate generated configs against downloaded release binaries through `NEXAGATE_TEST_XRAY`, `NEXAGATE_TEST_HYSTERIA`, and `NEXAGATE_TEST_PSIPHON`.

## License

MIT. Third-party programs retain their own licenses and terms. NexaGate is not affiliated with Psiphon, Cloudflare, XTLS/Xray, Hysteria, Let's Encrypt, or Certbot.
[🦁☀️ فارسی](README.fa.md) · [🇬🇧 English](README.md) · [🇷🇺 Русский](README.ru.md) · [🇨🇳 简体中文](README.zh-CN.md) · [🇸🇦 العربية](README.ar.md)
