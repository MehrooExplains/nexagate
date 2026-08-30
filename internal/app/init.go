package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

type InitOptions struct {
	ConfigPath            string
	StatePath             string
	GeneratedDir          string
	Listen                string
	PublicHost            string
	CertName              string
	ACMEWebroot           string
	AdminPassword         string
	AdminPasswordFile     string
	RealityPrivateKey     string
	RealityPrivateKeyFile string
	RealityPublicKey      string
	RealityTarget         string
	RealityShortID        string
	HysteriaObfs          string
	XHTTPPath             string
	XHTTPTLSPath          string
	WebSocketPath         string
}

func Initialize(opts InitOptions) error {
	if opts.AdminPassword == "" && opts.AdminPasswordFile != "" {
		value, err := os.ReadFile(opts.AdminPasswordFile)
		if err != nil {
			return fmt.Errorf("read administrator password: %w", err)
		}
		opts.AdminPassword = strings.TrimSpace(string(value))
	}
	if opts.RealityPrivateKey == "" && opts.RealityPrivateKeyFile != "" {
		value, err := os.ReadFile(opts.RealityPrivateKeyFile)
		if err != nil {
			return fmt.Errorf("read REALITY private key: %w", err)
		}
		opts.RealityPrivateKey = strings.TrimSpace(string(value))
	}
	if strings.TrimSpace(opts.PublicHost) == "" || strings.ContainsAny(opts.PublicHost, "/ ") {
		return errors.New("--host must be a domain name or IP address")
	}
	if net.ParseIP(opts.PublicHost) == nil && !validDomain(opts.PublicHost) {
		return errors.New("--host is not a valid domain or IP address")
	}
	if opts.CertName == "" {
		opts.CertName = opts.PublicHost
	}
	if opts.AdminPassword == "" || opts.RealityPrivateKey == "" || opts.RealityPublicKey == "" {
		return errors.New("admin password and both REALITY keys are required")
	}
	adminHash, err := hashPassword(opts.AdminPassword)
	if err != nil {
		return err
	}
	sessionKey, err := randomBytes(32)
	if err != nil {
		return err
	}
	if opts.RealityShortID == "" {
		opts.RealityShortID, err = randomHex(8)
		if err != nil {
			return err
		}
	}
	if opts.HysteriaObfs == "" {
		opts.HysteriaObfs, err = randomToken(24)
		if err != nil {
			return err
		}
	}
	if opts.XHTTPPath == "" {
		path, e := randomHex(8)
		if e != nil {
			return e
		}
		opts.XHTTPPath = "/" + path
	}
	if opts.XHTTPTLSPath == "" {
		path, e := randomHex(12)
		if e != nil {
			return e
		}
		opts.XHTTPTLSPath = "/xhttp-tls-" + path
	}
	if opts.WebSocketPath == "" {
		path, e := randomHex(8)
		if e != nil {
			return e
		}
		opts.WebSocketPath = "/ws-" + path
	}
	if !validProxyPath(opts.XHTTPPath) || !validProxyPath(opts.XHTTPTLSPath) || !validProxyPath(opts.WebSocketPath) {
		return errors.New("XHTTP and WebSocket paths must be safe absolute URL paths")
	}
	if opts.XHTTPPath == opts.XHTTPTLSPath || opts.XHTTPTLSPath == opts.WebSocketPath || opts.XHTTPPath == opts.WebSocketPath {
		return errors.New("XHTTP and WebSocket paths must be unique")
	}

	certDir := filepath.Join("/etc/letsencrypt/live", opts.CertName)
	cfg := Config{
		Listen: opts.Listen, PublicHost: opts.PublicHost, CertName: opts.CertName,
		CertFile: filepath.Join(certDir, "fullchain.pem"), KeyFile: filepath.Join(certDir, "privkey.pem"),
		ACMEWebroot: opts.ACMEWebroot,
		StatePath:   opts.StatePath, GeneratedDir: opts.GeneratedDir,
		AdminHash: adminHash, SessionKey: base64.RawStdEncoding.EncodeToString(sessionKey),
		Ports: Ports{Hysteria: 443, XHTTPReality: 443, XHTTPTLS: 2053, XHTTPTLSLocal: 10002,
			RawReality: 8444, WebSocketTLS: 2053, WebSocketLocal: 10001, PanelHTTPS: 8443},
		Reality: RealityConfig{PrivateKey: opts.RealityPrivateKey, PublicKey: opts.RealityPublicKey,
			Target: opts.RealityTarget, ShortID: opts.RealityShortID, XHTTPPath: opts.XHTTPPath},
		Hysteria: HyConfig{ObfsPassword: opts.HysteriaObfs},
		Psiphon:  PsiphonConfig{SOCKSPort: 1080, Mode: "auto"},
		WARP:     WARPConfig{Interface: "warp0"}, XHTTPTLSPath: opts.XHTTPTLSPath, WebSocketPath: opts.WebSocketPath,
	}
	cfg.Hysteria.StatsSecret, err = randomToken(24)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(opts.GeneratedDir, 0750); err != nil {
		return err
	}
	if err := saveJSONAtomic(opts.ConfigPath, cfg, 0600); err != nil {
		return err
	}
	if _, err := os.Stat(opts.StatePath); errors.Is(err, os.ErrNotExist) {
		if err := saveJSONAtomic(opts.StatePath, Database{Users: []User{}}, 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := Render(cfg); err != nil {
		return err
	}
	fmt.Printf("NexaGate initialized for %s\n", opts.PublicHost)
	return nil
}

func validDomain(value string) bool {
	if len(value) > 253 || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(strings.ToLower(value), ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return strings.Contains(value, ".")
}

func validProxyPath(value string) bool {
	if len(value) < 2 || len(value) > 160 || value[0] != '/' || strings.Contains(value, "//") {
		return false
	}
	for _, char := range value[1:] {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("-._~/", char) {
			continue
		}
		return false
	}
	return true
}

func decodeConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := jsonUnmarshalStrict(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Listen == "" || cfg.PublicHost == "" || cfg.StatePath == "" || cfg.GeneratedDir == "" {
		return Config{}, errors.New("configuration is incomplete")
	}
	return cfg, nil
}

func normalizeConfig(cfg *Config) (bool, error) {
	changed := false
	if cfg.Ports.XHTTPTLS == 0 {
		cfg.Ports.XHTTPTLS = cfg.Ports.WebSocketTLS
		if cfg.Ports.XHTTPTLS == 0 {
			cfg.Ports.XHTTPTLS = 2053
		}
		changed = true
	}
	if cfg.Ports.XHTTPTLSLocal == 0 {
		cfg.Ports.XHTTPTLSLocal = 10002
		changed = true
	}
	if cfg.XHTTPTLSPath == "" {
		base := strings.TrimSuffix(cfg.Reality.XHTTPPath, "/")
		if base == "" {
			base = "/xhttp"
		}
		cfg.XHTTPTLSPath = base + "-tls"
		if cfg.XHTTPTLSPath == cfg.WebSocketPath {
			cfg.XHTTPTLSPath = base + "-secure"
		}
		changed = true
	}
	for name, value := range map[string]string{
		"REALITY XHTTP path": cfg.Reality.XHTTPPath,
		"TLS XHTTP path":     cfg.XHTTPTLSPath,
		"WebSocket path":     cfg.WebSocketPath,
	} {
		if !validProxyPath(value) {
			return false, fmt.Errorf("%s is invalid", name)
		}
	}
	if cfg.Reality.XHTTPPath == cfg.XHTTPTLSPath || cfg.XHTTPTLSPath == cfg.WebSocketPath || cfg.Reality.XHTTPPath == cfg.WebSocketPath {
		return false, errors.New("XHTTP and WebSocket paths must be unique")
	}
	for name, port := range map[string]int{
		"Hysteria": cfg.Ports.Hysteria, "XHTTP REALITY": cfg.Ports.XHTTPReality,
		"XHTTP TLS": cfg.Ports.XHTTPTLS, "XHTTP TLS local": cfg.Ports.XHTTPTLSLocal,
		"RAW REALITY": cfg.Ports.RawReality, "WebSocket TLS": cfg.Ports.WebSocketTLS,
		"WebSocket local": cfg.Ports.WebSocketLocal, "panel HTTPS": cfg.Ports.PanelHTTPS,
	} {
		if port < 1 || port > 65535 {
			return false, fmt.Errorf("%s port is outside 1-65535", name)
		}
	}
	if cfg.Ports.XHTTPTLSLocal == cfg.Ports.WebSocketLocal || cfg.Ports.XHTTPTLSLocal == cfg.Psiphon.SOCKSPort {
		return false, errors.New("XHTTP TLS local port conflicts with another local service")
	}
	if cfg.Ports.XHTTPTLS == cfg.Ports.PanelHTTPS || cfg.Ports.XHTTPTLS == cfg.Ports.XHTTPReality || cfg.Ports.XHTTPTLS == cfg.Ports.RawReality {
		return false, errors.New("XHTTP TLS public port conflicts with another TCP listener")
	}
	return changed, nil
}

func loadConfig(path string) (Config, error) {
	cfg, err := decodeConfig(path)
	if err != nil {
		return Config{}, err
	}
	if _, err := normalizeConfig(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadConfigForCLI exposes validated configuration loading to the management CLI.
func LoadConfigForCLI(path string) (Config, error) { return loadConfig(path) }

func loadConfigAndMigrate(path string) (Config, bool, error) {
	cfg, err := decodeConfig(path)
	if err != nil {
		return Config{}, false, err
	}
	changed, err := normalizeConfig(&cfg)
	if err != nil {
		return Config{}, false, err
	}
	return cfg, changed, nil
}
