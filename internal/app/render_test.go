package app

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanelTemplatesParse(t *testing.T) {
	if _, err := parsePageTemplates(); err != nil {
		t.Fatal(err)
	}
}

func TestDashboardLanguagesAreSeparatedAndIPIsPrivateByDefault(t *testing.T) {
	templates, err := parsePageTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		lang, direction, present, absent string
	}{
		{lang: "fa", direction: "rtl", present: "نمای کلی", absent: ">Overview<"},
		{lang: "en", direction: "ltr", present: "Overview", absent: "نمای کلی"},
	} {
		var output bytes.Buffer
		data := pageData{Title: "NexaGate", Lang: test.lang, CurrentPath: "/", ActiveNav: "overview", Update: updateStatus{State: "idle"}}
		if err := templates.ExecuteTemplate(&output, "dashboard", data); err != nil {
			t.Fatalf("render %s: %v", test.lang, err)
		}
		html := output.String()
		for _, required := range []string{`lang="` + test.lang + `"`, `dir="` + test.direction + `"`, test.present, `id="ip-visibility"`, `class="info-value ip-value"`} {
			if !strings.Contains(html, required) {
				t.Errorf("%s dashboard is missing %q", test.lang, required)
			}
		}
		for _, icon := range []string{"gauge", "users", "download", "upload", "route", "settings", "globe-2", "log-out", "cpu", "memory-stick", "arrow-right-left", "hard-drive", "refresh-cw", "eye-off", "eye"} {
			if !strings.Contains(html, `data-lucide="`+icon+`"`) {
				t.Errorf("%s dashboard is missing Lucide icon %q", test.lang, icon)
			}
		}
		if strings.Contains(html, test.absent) {
			t.Errorf("%s dashboard contains text from the other language: %q", test.lang, test.absent)
		}
	}
}

func TestLucideIconsHaveOneVisualSystem(t *testing.T) {
	for _, required := range []string{
		`.lucide-icon{display:block;fill:none;stroke:currentColor;stroke-linecap:round;stroke-linejoin:round}`,
		`.nav svg{width:20px;height:20px;stroke-width:1.8;flex:0 0 auto}`,
		`aria-hidden="true" focusable="false"`,
	} {
		if !strings.Contains(pageTemplates, required) {
			t.Errorf("page templates are missing the Lucide invariant %q", required)
		}
	}
}

func testConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	state := filepath.Join(dir, "users.json")
	if err := saveJSONAtomic(state, Database{Users: []User{{
		ID: "id", Name: "alice", Password: "password", UUID: "6ba7b810-9dad-41d1-80b4-00c04fd430c8", Enabled: true,
		CreatedAt: time.Now().UTC(),
	}}}, 0600); err != nil {
		t.Fatal(err)
	}
	return Config{
		Listen: "127.0.0.1:9080", PublicHost: "vpn.example.com", CertName: "vpn.example.com",
		CertFile: filepath.Join(dir, "cert.pem"), KeyFile: filepath.Join(dir, "key.pem"), ACMEWebroot: filepath.Join(dir, "webroot"),
		StatePath: state, GeneratedDir: filepath.Join(dir, "generated"), AdminHash: "test", SessionKey: "test",
		Ports: Ports{Hysteria: 443, XHTTPReality: 443, RawReality: 8444, WebSocketTLS: 2053, WebSocketLocal: 10001, PanelHTTPS: 8443},
		Reality: RealityConfig{PrivateKey: "MGS-scauOEJer1nkmHCQ5mgJnT-PeWR3QYaivMuPPGM", PublicKey: "buV-CvDpcbEd9hxJdWHNQLNDW0NQBzjlWeHDz232vUc",
			Target: "www.microsoft.com", ShortID: "0123456789abcdef", XHTTPPath: "/xhttp-test"},
		Hysteria: HyConfig{ObfsPassword: "obfs-test", StatsSecret: "stats-test"},
		Psiphon:  PsiphonConfig{SOCKSPort: 1080, Mode: "auto"}, WARP: WARPConfig{Interface: "warp0"}, WebSocketPath: "/ws-test",
	}
}

func TestRenderIsFailClosed(t *testing.T) {
	cfg := testConfig(t)
	if err := Render(cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"xray.json", "hysteria.yaml", "psiphon.json", "nginx.conf"} {
		info, err := os.Stat(filepath.Join(cfg.GeneratedDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("%s has permissions %04o, want 0600", name, info.Mode().Perm())
		}
	}
	xray, err := os.ReadFile(filepath.Join(cfg.GeneratedDir, "xray.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"outboundTag": "psiphon"`, `"outboundTag": "warp"`, `"outboundTag": "blocked"`, `"interface": "warp0"`} {
		if !strings.Contains(string(xray), required) {
			t.Errorf("Xray config is missing %s", required)
		}
	}
	hy, err := os.ReadFile(filepath.Join(cfg.GeneratedDir, "hysteria.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"psiphon(all, tcp)", "warp(all, udp)", "reject(all)", "bindDevice: 'warp0'"} {
		if !strings.Contains(string(hy), required) {
			t.Errorf("Hysteria config is missing %s", required)
		}
	}
	psi, err := os.ReadFile(filepath.Join(cfg.GeneratedDir, "psiphon.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(psi), `"EgressRegion": ""`) {
		t.Error("automatic Psiphon mode did not render an empty EgressRegion")
	}
}

func TestGeneratedConfigsWithReleaseBinaries(t *testing.T) {
	xray := os.Getenv("NEXAGATE_TEST_XRAY")
	hysteria := os.Getenv("NEXAGATE_TEST_HYSTERIA")
	if xray == "" || hysteria == "" {
		t.Skip("release binaries were not supplied")
	}
	cfg := testConfig(t)
	cfg.WARP.Interface = "lo"
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("openssl is unavailable")
	}
	certCommand := exec.Command(openssl, "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1", "-subj", "/CN=vpn.example.com", "-keyout", cfg.KeyFile, "-out", cfg.CertFile)
	if output, err := certCommand.CombinedOutput(); err != nil {
		t.Fatalf("generate test certificate: %v: %s", err, output)
	}
	if err := Render(cfg); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(xray, "run", "-test", "-config", filepath.Join(cfg.GeneratedDir, "xray.json")).CombinedOutput(); err != nil {
		t.Fatalf("Xray rejected generated config: %v: %s", err, output)
	}
	hyPath := filepath.Join(cfg.GeneratedDir, "hysteria.yaml")
	hyData, err := os.ReadFile(hyPath)
	if err != nil {
		t.Fatal(err)
	}
	hyData = []byte(strings.Replace(string(hyData), "listen: :443", "listen: :0", 1))
	hyData = []byte(strings.Replace(string(hyData), "listen: 127.0.0.1:9999", "listen: 127.0.0.1:0", 1))
	if err := os.WriteFile(hyPath, hyData, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, hysteria, "server", "--config", hyPath, "--disable-update-check")
	output, runErr := command.CombinedOutput()
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("Hysteria rejected generated config: %v: %s", runErr, output)
	}
	if psiphon := os.Getenv("NEXAGATE_TEST_PSIPHON"); psiphon != "" {
		psiContext, psiCancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer psiCancel()
		psiDataDir := filepath.Join(filepath.Dir(cfg.GeneratedDir), "psiphon")
		if err := os.MkdirAll(psiDataDir, 0700); err != nil {
			t.Fatal(err)
		}
		psiCommand := exec.CommandContext(psiContext, psiphon, "-config", filepath.Join(cfg.GeneratedDir, "psiphon.json"),
			"-dataRootDirectory", psiDataDir, "-listenInterface", "lo")
		psiOutput, psiErr := psiCommand.CombinedOutput()
		if psiContext.Err() != context.DeadlineExceeded {
			t.Fatalf("Psiphon rejected generated config: %v: %s", psiErr, psiOutput)
		}
	}
}
