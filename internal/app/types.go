package app

import "time"

type Config struct {
	Listen        string        `json:"listen"`
	PublicHost    string        `json:"public_host"`
	CertName      string        `json:"cert_name"`
	CertFile      string        `json:"cert_file"`
	KeyFile       string        `json:"key_file"`
	ACMEWebroot   string        `json:"acme_webroot"`
	StatePath     string        `json:"state_path"`
	GeneratedDir  string        `json:"generated_dir"`
	AdminHash     string        `json:"admin_hash"`
	SessionKey    string        `json:"session_key"`
	Ports         Ports         `json:"ports"`
	Reality       RealityConfig `json:"reality"`
	Hysteria      HyConfig      `json:"hysteria"`
	Psiphon       PsiphonConfig `json:"psiphon"`
	WARP          WARPConfig    `json:"warp"`
	XHTTPTLSPath  string        `json:"xhttp_tls_path"`
	WebSocketPath string        `json:"websocket_path"`
}

type Ports struct {
	Hysteria       int `json:"hysteria"`
	XHTTPReality   int `json:"xhttp_reality"`
	XHTTPTLS       int `json:"xhttp_tls"`
	XHTTPTLSLocal  int `json:"xhttp_tls_local"`
	RawReality     int `json:"raw_reality"`
	WebSocketTLS   int `json:"websocket_tls"`
	WebSocketLocal int `json:"websocket_local"`
	PanelHTTPS     int `json:"panel_https"`
}

type RealityConfig struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
	Target     string `json:"target"`
	ShortID    string `json:"short_id"`
	XHTTPPath  string `json:"xhttp_path"`
}

type HyConfig struct {
	ObfsPassword string `json:"obfs_password"`
	StatsSecret  string `json:"stats_secret"`
}

type PsiphonConfig struct {
	SOCKSPort int    `json:"socks_port"`
	Mode      string `json:"mode"`
	Region    string `json:"region"`
}

type WARPConfig struct {
	Interface string `json:"interface"`
}

type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Password  string    `json:"password"`
	UUID      string    `json:"uuid"`
	Enabled   bool      `json:"enabled"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Database struct {
	Users     []User    `json:"users"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (u User) Active(now time.Time) bool {
	return u.Enabled && (u.ExpiresAt.IsZero() || u.ExpiresAt.After(now))
}
