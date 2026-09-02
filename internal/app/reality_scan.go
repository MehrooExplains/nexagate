package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

var commonRealityTargets = []string{
	"www.samsung.com", "www.nvidia.com", "www.cloudflare.com",
	"www.amd.com", "dl.google.com", "www.microsoft.com",
}

type realityScanResult struct {
	Target      string `json:"target"`
	Status      string `json:"status"`
	Selectable  bool   `json:"selectable"`
	TLS         string `json:"tls"`
	ALPN        string `json:"alpn"`
	KeyExchange string `json:"key_exchange"`
	Certificate string `json:"certificate"`
	LatencyMS   int64  `json:"latency_ms"`
	Error       string `json:"error,omitempty"`
}

func scanRealityTargets(ctx context.Context, input string) ([]realityScanResult, error) {
	targets, err := realityTargets(strings.TrimSpace(input))
	if err != nil {
		return nil, err
	}
	results := make([]realityScanResult, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target string) {
			defer wg.Done()
			results[i] = probeRealityTarget(ctx, target)
		}(i, target)
	}
	wg.Wait()
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Status != results[j].Status {
			return results[i].Status == "feasible"
		}
		return results[i].LatencyMS < results[j].LatencyMS
	})
	return results, nil
}

func realityTargets(input string) ([]string, error) {
	if input == "" {
		return append([]string(nil), commonRealityTargets...), nil
	}
	if _, network, err := net.ParseCIDR(input); err == nil {
		ones, bits := network.Mask.Size()
		if bits != 32 || ones < 28 {
			return nil, errors.New("IPv4 CIDR must be /28 or smaller")
		}
		var out []string
		for ip := network.IP.Mask(network.Mask); network.Contains(ip); ip = nextIPv4(ip) {
			if isPublicIP(ip) {
				out = append(out, ip.String())
			}
			if len(out) > 16 {
				return nil, errors.New("CIDR contains more than 16 public addresses")
			}
		}
		if len(out) == 0 {
			return nil, errors.New("CIDR contains no public addresses")
		}
		return out, nil
	}
	if ip := net.ParseIP(input); ip != nil {
		if !isPublicIP(ip) {
			return nil, errors.New("private, loopback, and special IPs are not allowed")
		}
		return []string{ip.String()}, nil
	}
	if !validHostName(input) {
		return nil, errors.New("enter a valid domain, public IPv4, or IPv4 CIDR")
	}
	return []string{strings.ToLower(input)}, nil
}

func nextIPv4(ip net.IP) net.IP {
	next := append(net.IP(nil), ip...)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

func isPublicIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4"} {
		_, network, _ := net.ParseCIDR(block)
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

func validHostName(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func probeRealityTarget(parent context.Context, target string) realityScanResult {
	result := realityScanResult{
		Target:      net.JoinHostPort(target, "443"),
		Status:      "unavailable",
		Selectable:  net.ParseIP(target) == nil,
		KeyExchange: "X25519",
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, target)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	var dialIP net.IP
	for _, address := range addresses {
		if address.IP.To4() == nil {
			continue
		}
		if !isPublicIP(address.IP) {
			result.Error = "target resolves to a non-public address"
			return result
		}
		if dialIP == nil {
			dialIP = address.IP.To4()
		}
	}
	if dialIP == nil {
		result.Error = "target has no public IPv4 address"
		return result
	}
	serverName := target
	insecure := net.ParseIP(target) != nil
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	start := time.Now()
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(dialIP.String(), "443"), &tls.Config{
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS12,
		CurvePreferences:   []tls.CurveID{tls.X25519},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: insecure, // IP discovery still inspects the presented certificate.
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer conn.Close()
	state := conn.ConnectionState()
	result.LatencyMS = time.Since(start).Milliseconds()
	result.TLS = fmt.Sprintf("0x%x", state.Version)
	if state.Version == tls.VersionTLS13 {
		result.TLS = "1.3"
	} else if state.Version == tls.VersionTLS12 {
		result.TLS = "1.2"
	}
	result.ALPN = state.NegotiatedProtocol
	if result.ALPN == "" {
		result.ALPN = "none"
	}
	if len(state.PeerCertificates) > 0 {
		cert := state.PeerCertificates[0]
		result.Certificate = cert.Subject.CommonName
		if result.Certificate == "" && len(cert.DNSNames) > 0 {
			result.Certificate = cert.DNSNames[0]
		}
	}
	if state.Version == tls.VersionTLS13 && result.Certificate != "" {
		result.Status = "feasible"
	}
	return result
}
