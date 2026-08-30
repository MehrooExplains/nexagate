package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func RenderFromFile(path string) error {
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}
	return Render(cfg)
}

func Render(cfg Config) error {
	db, err := loadDatabase(cfg.StatePath)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	users := make([]User, 0, len(db.Users))
	for _, user := range db.Users {
		if user.Active(now) {
			users = append(users, user)
		}
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })

	xray, err := renderXray(cfg, users)
	if err != nil {
		return err
	}
	hysteria := renderHysteria(cfg, users)
	nginx := renderNginx(cfg)
	psiphon, err := renderPsiphon(cfg)
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(cfg.GeneratedDir, "xray.json"), xray, 0600); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(cfg.GeneratedDir, "hysteria.yaml"), []byte(hysteria), 0600); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(cfg.GeneratedDir, "nginx.conf"), []byte(nginx), 0600); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(cfg.GeneratedDir, "psiphon.json"), psiphon, 0600); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(cfg.GeneratedDir, ".reload"), []byte(now.Format(time.RFC3339Nano)+"\n"), 0640)
}

func renderPsiphon(cfg Config) ([]byte, error) {
	config := map[string]any{
		"LocalSocksProxyPort":                cfg.Psiphon.SOCKSPort,
		"LocalHttpProxyPort":                 8082,
		"EgressRegion":                       strings.ToUpper(strings.TrimSpace(cfg.Psiphon.Region)),
		"PropagationChannelId":               "FFFFFFFFFFFFFFFF",
		"RemoteServerListSignaturePublicKey": "MIICIDANBgkqhkiG9w0BAQEFAAOCAg0AMIICCAKCAgEAt7Ls+/39r+T6zNW7GiVpJfzq/xvL9SBH5rIFnk0RXYEYavax3WS6HOD35eTAqn8AniOwiH+DOkvgSKF2caqk/y1dfq47Pdymtwzp9ikpB1C5OfAysXzBiwVJlCdajBKvBZDerV1cMvRzCKvKwRmvDmHgphQQ7WfXIGbRbmmk6opMBh3roE42KcotLFtqp0RRwLtcBRNtCdsrVsjiI1Lqz/lH+T61sGjSjQ3CHMuZYSQJZo/KrvzgQXpkaCTdbObxHqb6/+i1qaVOfEsvjoiyzTxJADvSytVtcTjijhPEV6XskJVHE1Zgl+7rATr/pDQkw6DPCNBS1+Y6fy7GstZALQXwEDN/qhQI9kWkHijT8ns+i1vGg00Mk/6J75arLhqcodWsdeG/M/moWgqQAnlZAGVtJI1OgeF5fsPpXu4kctOfuZlGjVZXQNW34aOzm8r8S0eVZitPlbhcPiR4gT/aSMz/wd8lZlzZYsje/Jr8u/YtlwjjreZrGRmG8KMOzukV3lLmMppXFMvl4bxv6YFEmIuTsOhbLTwFgh7KYNjodLj/LsqRVfwz31PgWQFTEPICV7GCvgVlPRxnofqKSjgTWI4mxDhBpVcATvaoBl1L/6WLbFvBsoAUBItWwctO2xalKxF5szhGm8lccoc5MZr8kfE0uxMgsxz4er68iCID+rsCAQM=",
		"RemoteServerListUrl":                "https://s3.amazonaws.com//psiphon/web/mjr4-p23r-puwl/server_list_compressed",
		"SponsorId":                          "FFFFFFFFFFFFFFFF",
		"EmitDiagnosticNotices":              false,
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func renderXray(cfg Config, users []User) ([]byte, error) {
	clients := make([]map[string]any, 0, len(users))
	rawClients := make([]map[string]any, 0, len(users))
	for _, user := range users {
		clients = append(clients, map[string]any{"id": user.UUID, "email": user.Name})
		rawClients = append(rawClients, map[string]any{"id": user.UUID, "email": user.Name, "flow": "xtls-rprx-vision"})
	}
	realityBase := map[string]any{
		"show": false, "target": net.JoinHostPort(cfg.Reality.Target, "443"), "xver": 0,
		"serverNames": []string{cfg.Reality.Target}, "privateKey": cfg.Reality.PrivateKey,
		"shortIds":              []string{cfg.Reality.ShortID},
		"limitFallbackUpload":   map[string]any{"afterBytes": 65536, "bytesPerSec": 16384, "burstBytesPerSec": 32768},
		"limitFallbackDownload": map[string]any{"afterBytes": 65536, "bytesPerSec": 16384, "burstBytesPerSec": 32768},
	}
	config := map[string]any{
		"log": map[string]any{"loglevel": "warning", "dnsLog": false},
		"dns": map[string]any{"servers": []any{"udp://127.0.0.1:1053"}, "queryStrategy": "UseIPv4", "tag": "dns-local"},
		"inbounds": []any{
			map[string]any{
				"tag": "vless-xhttp-reality", "listen": "0.0.0.0", "port": cfg.Ports.XHTTPReality, "protocol": "vless",
				"settings": map[string]any{"clients": clients, "decryption": "none"},
				"streamSettings": map[string]any{"method": "xhttp", "security": "reality", "realitySettings": realityBase,
					"xhttpSettings": map[string]any{"path": cfg.Reality.XHTTPPath, "mode": "auto"}},
				"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
			},
			map[string]any{
				"tag": "vless-xhttp-tls", "listen": "127.0.0.1", "port": cfg.Ports.XHTTPTLSLocal, "protocol": "vless",
				"settings": map[string]any{"clients": clients, "decryption": "none"},
				"streamSettings": map[string]any{"method": "xhttp", "security": "none",
					"xhttpSettings": map[string]any{"path": cfg.XHTTPTLSPath, "mode": "auto"}},
				"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
			},
			map[string]any{
				"tag": "vless-raw-reality", "listen": "0.0.0.0", "port": cfg.Ports.RawReality, "protocol": "vless",
				"settings":       map[string]any{"clients": rawClients, "decryption": "none"},
				"streamSettings": map[string]any{"method": "raw", "security": "reality", "realitySettings": realityBase},
				"sniffing":       map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
			},
			map[string]any{
				"tag": "vless-websocket-tls", "listen": "127.0.0.1", "port": cfg.Ports.WebSocketLocal, "protocol": "vless",
				"settings": map[string]any{"clients": clients, "decryption": "none"},
				"streamSettings": map[string]any{"method": "websocket", "security": "none",
					"wsSettings": map[string]any{"path": cfg.WebSocketPath}},
				"sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true},
			},
		},
		"outbounds": []any{
			map[string]any{"tag": "blocked", "protocol": "blackhole"},
			map[string]any{"tag": "psiphon", "protocol": "socks", "settings": map[string]any{
				"address": "127.0.0.1", "port": cfg.Psiphon.SOCKSPort}},
			map[string]any{"tag": "warp", "protocol": "freedom", "settings": map[string]any{
				"domainStrategy": "UseIPv4", "finalRules": []any{map[string]any{"action": "allow", "network": "udp"}, map[string]any{"action": "block"}}},
				"streamSettings": map[string]any{"sockopt": map[string]any{"interface": cfg.WARP.Interface, "domainStrategy": "UseIPv4"}}},
			map[string]any{"tag": "local-dns", "protocol": "freedom", "settings": map[string]any{
				"domainStrategy": "AsIs", "finalRules": []any{
					map[string]any{"action": "allow", "network": "udp", "ip": []string{"127.0.0.0/8"}, "port": "1053"},
					map[string]any{"action": "block"}}}},
		},
		"routing": map[string]any{"domainStrategy": "AsIs", "rules": []any{
			map[string]any{"inboundTag": []string{"dns-local"}, "outboundTag": "local-dns"},
			map[string]any{"network": "tcp", "outboundTag": "psiphon"},
			map[string]any{"network": "udp", "outboundTag": "warp"},
			map[string]any{"network": "tcp,udp", "outboundTag": "blocked"},
		}},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func renderHysteria(cfg Config, users []User) string {
	var b strings.Builder
	fmt.Fprintf(&b, "listen: :%d\n", cfg.Ports.Hysteria)
	sniGuard := "strict"
	if net.ParseIP(cfg.PublicHost) != nil {
		sniGuard = "disable"
	}
	fmt.Fprintf(&b, "tls:\n  cert: %s\n  key: %s\n  sniGuard: %s\n", yamlQuote(cfg.CertFile), yamlQuote(cfg.KeyFile), sniGuard)
	fmt.Fprintf(&b, "obfs:\n  type: salamander\n  salamander:\n    password: %s\n", yamlQuote(cfg.Hysteria.ObfsPassword))
	b.WriteString("congestion:\n  type: bbr\n  bbrProfile: standard\n")
	b.WriteString("auth:\n  type: userpass\n  userpass:\n")
	if len(users) == 0 {
		b.WriteString("    disabled: disabled\n")
	}
	for _, user := range users {
		fmt.Fprintf(&b, "    %s: %s\n", yamlQuote(user.Name), yamlQuote(user.Password))
	}
	b.WriteString("resolver:\n  type: udp\n  udp:\n    addr: 127.0.0.1:1053\n    timeout: 5s\n")
	fmt.Fprintf(&b, "outbounds:\n  - name: psiphon\n    type: socks5\n    socks5:\n      addr: 127.0.0.1:%d\n", cfg.Psiphon.SOCKSPort)
	fmt.Fprintf(&b, "  - name: warp\n    type: direct\n    direct:\n      mode: 4\n      bindDevice: %s\n", yamlQuote(cfg.WARP.Interface))
	b.WriteString("acl:\n  inline:\n    - psiphon(all, tcp)\n    - warp(all, udp)\n    - reject(all)\n")
	fmt.Fprintf(&b, "trafficStats:\n  listen: 127.0.0.1:9999\n  secret: %s\n", yamlQuote(cfg.Hysteria.StatsSecret))
	b.WriteString("masquerade:\n  type: string\n  string:\n    content: NexaGate\n    headers:\n      content-type: text/plain\n    statusCode: 200\n")
	return b.String()
}

func renderNginx(cfg Config) string {
	serverName := cfg.PublicHost
	var b strings.Builder
	fmt.Fprintf(&b, `# Generated by NexaGate. Local changes will be replaced.
server {
    listen 80;
    listen [::]:80;
    server_name %s;
    root %s;
    location ^~ /.well-known/acme-challenge/ { try_files $uri =404; }
    location / { return 301 https://$host:%d$request_uri; }
}

server {
    listen %d ssl;
    listen [::]:%d ssl;
    server_name %s;
    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
    add_header Strict-Transport-Security "max-age=31536000" always;
    location / {
        proxy_pass http://127.0.0.1:9080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
`, serverName, cfg.ACMEWebroot, cfg.Ports.PanelHTTPS, cfg.Ports.PanelHTTPS, cfg.Ports.PanelHTTPS, serverName,
		cfg.CertFile, cfg.KeyFile)
	if cfg.Ports.XHTTPTLS == cfg.Ports.WebSocketTLS {
		renderNginxTransportServer(&b, cfg, cfg.Ports.XHTTPTLS, true, true)
	} else {
		renderNginxTransportServer(&b, cfg, cfg.Ports.XHTTPTLS, true, false)
		renderNginxTransportServer(&b, cfg, cfg.Ports.WebSocketTLS, false, true)
	}
	return b.String()
}

func renderNginxTransportServer(b *strings.Builder, cfg Config, port int, includeXHTTP, includeWebSocket bool) {
	fmt.Fprintf(b, `
server {
    listen %d ssl http2;
    listen [::]:%d ssl http2;
    server_name %s;
    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
`, port, port, cfg.PublicHost, cfg.CertFile, cfg.KeyFile)
	if includeXHTTP {
		path := strings.TrimSuffix(cfg.XHTTPTLSPath, "/")
		fmt.Fprintf(b, `    location ^~ %s/ {
        access_log off;
        client_max_body_size 2m;
        grpc_buffer_size 16k;
        grpc_connect_timeout 10s;
        grpc_read_timeout 1d;
        grpc_send_timeout 1d;
        grpc_socket_keepalive on;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
        grpc_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        grpc_pass grpc://127.0.0.1:%d;
    }
`, path, cfg.Ports.XHTTPTLSLocal)
	}
	if includeWebSocket {
		fmt.Fprintf(b, `    location = %s {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 1d;
    }
`, cfg.WebSocketPath, cfg.Ports.WebSocketLocal)
	}
	b.WriteString("    location / { return 404; }\n}\n")
}

func yamlQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

type Links struct{ Hysteria, XHTTPReality, XHTTPTLS, RawReality, WebSocketTLS string }

func userLinks(cfg Config, user User) Links {
	host := cfg.PublicHost
	authority := net.JoinHostPort(host, fmt.Sprint(cfg.Ports.Hysteria))
	label := url.QueryEscape("NexaGate-" + user.Name)
	hyQuery := url.Values{"sni": {host}, "obfs": {"salamander"}, "obfs-password": {cfg.Hysteria.ObfsPassword}}
	hy := "hysteria2://" + url.PathEscape(user.Name) + ":" + url.PathEscape(user.Password) + "@" + authority + "/?" + hyQuery.Encode() + "#" + label
	baseReality := url.Values{"security": {"reality"}, "encryption": {"none"}, "pbk": {cfg.Reality.PublicKey},
		"fp": {"chrome"}, "sni": {cfg.Reality.Target}, "sid": {cfg.Reality.ShortID}}
	xq := cloneValues(baseReality)
	xq.Set("type", "xhttp")
	xq.Set("path", cfg.Reality.XHTTPPath)
	xq.Set("mode", "auto")
	tq := url.Values{"type": {"xhttp"}, "security": {"tls"}, "encryption": {"none"}, "sni": {host},
		"host": {host}, "path": {cfg.XHTTPTLSPath}, "mode": {"auto"}, "alpn": {"h2"}, "fp": {"chrome"}}
	rq := cloneValues(baseReality)
	rq.Set("type", "tcp")
	rq.Set("flow", "xtls-rprx-vision")
	wq := url.Values{"type": {"ws"}, "security": {"tls"}, "encryption": {"none"}, "sni": {host}, "host": {host}, "path": {cfg.WebSocketPath}}
	return Links{
		Hysteria:     hy,
		XHTTPReality: vlessLink(user.UUID, host, cfg.Ports.XHTTPReality, xq, "XHTTP-"+user.Name),
		XHTTPTLS:     vlessLink(user.UUID, host, cfg.Ports.XHTTPTLS, tq, "XHTTP-TLS-"+user.Name),
		RawReality:   vlessLink(user.UUID, host, cfg.Ports.RawReality, rq, "REALITY-"+user.Name),
		WebSocketTLS: vlessLink(user.UUID, host, cfg.Ports.WebSocketTLS, wq, "WS-TLS-"+user.Name),
	}
}

func cloneValues(source url.Values) url.Values {
	out := url.Values{}
	for k, values := range source {
		out[k] = append([]string(nil), values...)
	}
	return out
}
func vlessLink(uuid, host string, port int, query url.Values, label string) string {
	return "vless://" + uuid + "@" + net.JoinHostPort(host, fmt.Sprint(port)) + "?" + query.Encode() + "#" + url.QueryEscape(label)
}

func readConfigFile(path string) ([]byte, error) { return os.ReadFile(path) }
