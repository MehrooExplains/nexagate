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
	if opts.WebSocketPath == "" {
		path, e := randomHex(8)
		if e != nil {
			return e
		}
		opts.WebSocketPath = "/ws-" + path
	}
	if !strings.HasPrefix(opts.XHTTPPath, "/") || !strings.HasPrefix(opts.WebSocketPath, "/") {
		return errors.New("XHTTP and WebSocket paths must begin with /")
	}

	certDir := filepath.Join("/etc/letsencrypt/live", opts.CertName)
	cfg := Config{
		Listen: opts.Listen, PublicHost: opts.PublicHost, CertName: opts.CertName,
		CertFile: filepath.Join(certDir, "fullchain.pem"), KeyFile: filepath.Join(certDir, "privkey.pem"),
		ACMEWebroot: opts.ACMEWebroot,
		StatePath:   opts.StatePath, GeneratedDir: opts.GeneratedDir,
		AdminHash: adminHash, SessionKey: base64.RawStdEncoding.EncodeToString(sessionKey),
		Ports: Ports{Hysteria: 443, XHTTPReality: 443, RawReality: 8444, WebSocketTLS: 2053, WebSocketLocal: 10001, PanelHTTPS: 8443},
		Reality: RealityConfig{PrivateKey: opts.RealityPrivateKey, PublicKey: opts.RealityPublicKey,
			Target: opts.RealityTarget, ShortID: opts.RealityShortID, XHTTPPath: opts.XHTTPPath},
		Hysteria: HyConfig{ObfsPassword: opts.HysteriaObfs},
		Psiphon:  PsiphonConfig{SOCKSPort: 1080, Mode: "auto"},
		WARP:     WARPConfig{Interface: "warp0"}, WebSocketPath: opts.WebSocketPath,
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

func loadConfig(path string) (Config, error) {
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
