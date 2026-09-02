# NexaGate

轻量级控制面板，支持 Hysteria2、VLESS XHTTP Reality、VLESS XHTTP TLS、RAW Reality 和 WebSocket TLS。

## 快速开始

安装程序会自动检查并安装依赖：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MehrooExplains/nexagate/main/install.sh)
```

安装后打开 HTTPS 面板，使用用户链接或二维码。TCP 流量通过 Psiphon，UDP 流量通过 WARP。旧配置会自动、安全地迁移。

完整文档请查看[英文版](README.md)或[波斯语版](README.fa.md)。

## 详细说明

NexaGate 会刻意分离出站路径：隧道内 TCP 通过 Psiphon 出站，UDP 和 DNS 通过 Cloudflare WARP 出站。fail-closed 防火墙防止 Xray、Hysteria 和 DNS 中继在隧道故障时静默改用服务器普通公网路由。

> 版本：`0.3.0`。请先在可恢复的新服务器上测试。没有任何协议可以保证在全部网络过滤条件下均可访问。

### 架构与端口

```text
客户端
  ├─ UDP/443  ─ Hysteria2
  ├─ TCP/443  ─ VLESS + XHTTP + REALITY
  ├─ TCP/8444 ─ VLESS + RAW + REALITY + Vision
  └─ TCP/2053 ─ Nginx TLS/HTTP2
                    ├─ VLESS + XHTTP + TLS
                    └─ VLESS + WebSocket + TLS

管理：TCP/8443 → Nginx HTTPS → 127.0.0.1:9080 上的面板
```

| 端口 | 用途 |
|---|---|
| UDP `443` | Hysteria2 + Salamander |
| TCP `443` | VLESS XHTTP + REALITY |
| TCP `8444` | VLESS RAW + REALITY + Vision |
| TCP `2053` | VLESS XHTTP TLS 和 WebSocket TLS |
| TCP `8443` | HTTPS 管理面板 |
| TCP `80` | 证书 HTTP-01 |

TCP 与 UDP 可同时使用 `443`，因为二者是不同的传输协议。请在服务器防火墙和云安全组中开放 TCP `80,443,2053,8443,8444` 与 UDP `443`。

### 安装

需要新的 systemd Linux 服务器、`amd64`/`x86_64`、公网 IPv4 和 root/`sudo` 权限。支持 `apt`、`dnf` 和 `yum`，其中 Debian/Ubuntu 测试最充分。安装程序会在需要时提权，检查依赖，并只安装缺失的软件包。

安装仅询问证书类型、Let's Encrypt 邮箱和域名模式下的域名。系统会自动生成高强度随机管理员密码，只在完成时显示一次，并保存到仅 root 可读的 `/root/nexagate-initial-credentials.txt`。登录后请在 **Panel Settings** 修改密码并删除该文件。

### REALITY 目标扫描器

在 **Panel Settings → REALITY Target Scanner** 中打开。空输入会扫描内置候选域名；也可扫描一个域名或 `/28` 或更小的公网 IPv4 CIDR（最多 16 个地址）。扫描器从服务器端测量 TLS、ALPN、证书、X25519 偏好与延迟，并拒绝扫描私有和特殊用途网络。

IP/CIDR 结果仅供参考；只有已验证的域名才显示 **Use**，因为 REALITY 需要匹配的 SNI 主机名。`feasible` 结果仅代表 TLS 兼容，不能保证在所有网络环境中可用。

### 面板与安全性

- 面板显示 CPU、内存、swap、磁盘、网络速度/流量、套接字、运行时间和面板资源；服务器 IP 默认隐藏，点击眼睛图标才显示。
- 支持用户、订阅链接、二维码、Psiphon/WARP 自动选择、带 checksum 与回滚的一键更新，以及 [CertDuo](https://github.com/MehrooExplains/certduo) 证书。
- 管理员密码使用带随机盐的 PBKDF2-HMAC-SHA-256；会话为 Secure、HTTP-only 和 SameSite strict，并有 CSRF 与登录限速保护。
- 连接链接含凭据，不应公开；如泄露，请禁用或删除用户。

```bash
sudo nexagate doctor
sudo nexagate backup create --encrypt --password
sudo nexagate backup list
```
