package app

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func Doctor(configPath string, output io.Writer) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	checks := []struct {
		name   string
		ok     bool
		detail string
	}{
		{"configuration", true, configPath},
		{"certificate", fileReadable(cfg.CertFile) && fileReadable(cfg.KeyFile), cfg.CertFile},
		{"user database", fileReadable(cfg.StatePath), cfg.StatePath},
		{"Xray config", fileReadable(filepath.Join(cfg.GeneratedDir, "xray.json")), filepath.Join(cfg.GeneratedDir, "xray.json")},
		{"Hysteria config", fileReadable(filepath.Join(cfg.GeneratedDir, "hysteria.yaml")), filepath.Join(cfg.GeneratedDir, "hysteria.yaml")},
		{"Psiphon config", fileReadable(filepath.Join(cfg.GeneratedDir, "psiphon.json")), filepath.Join(cfg.GeneratedDir, "psiphon.json")},
		{"Nginx config", fileReadable(filepath.Join(cfg.GeneratedDir, "nginx.conf")), filepath.Join(cfg.GeneratedDir, "nginx.conf")},
		{"WARP interface", interfaceExists(cfg.WARP.Interface), cfg.WARP.Interface},
		{"Psiphon SOCKS", tcpOpen(fmt.Sprintf("127.0.0.1:%d", cfg.Psiphon.SOCKSPort)), fmt.Sprintf("127.0.0.1:%d", cfg.Psiphon.SOCKSPort)},
		{"fail-closed firewall", commandOK("nft", "list", "table", "inet", "nexagate"), "inet/nexagate"},
	}
	for _, service := range []string{"nexagate-panel", "nexagate-xray", "nexagate-hysteria", "nexagate-psiphon", "wg-quick@warp0", "nginx"} {
		checks = append(checks, struct {
			name   string
			ok     bool
			detail string
		}{"service " + service, serviceActive(service), service})
	}
	failed := false
	for _, check := range checks {
		status := "OK"
		if !check.ok {
			status = "FAIL"
			failed = true
		}
		fmt.Fprintf(output, "%-5s %-24s %s\n", status, check.name, check.detail)
	}
	if failed {
		return fmt.Errorf("one or more checks failed")
	}
	return nil
}

func fileReadable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	return file.Close() == nil
}
func interfaceExists(name string) bool { _, err := net.InterfaceByName(name); return err == nil }
func tcpOpen(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 800*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
func serviceActive(name string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
}
func commandOK(name string, args ...string) bool { return exec.Command(name, args...).Run() == nil }
