package app

import (
	"bufio"
	"errors"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type metricsCollector struct {
	mu            sync.Mutex
	lastCPUIdle   uint64
	lastCPUTotal  uint64
	lastRX        uint64
	lastTX        uint64
	lastNetwork   time.Time
	interfaceName string
}

type liveMetrics struct {
	Timestamp        int64    `json:"timestamp"`
	CPUPercent       float64  `json:"cpu_percent"`
	CPUCores         int      `json:"cpu_cores"`
	Load1            float64  `json:"load_1"`
	Load5            float64  `json:"load_5"`
	Load15           float64  `json:"load_15"`
	MemoryUsed       uint64   `json:"memory_used"`
	MemoryTotal      uint64   `json:"memory_total"`
	MemoryPercent    float64  `json:"memory_percent"`
	SwapUsed         uint64   `json:"swap_used"`
	SwapTotal        uint64   `json:"swap_total"`
	SwapPercent      float64  `json:"swap_percent"`
	StorageUsed      uint64   `json:"storage_used"`
	StorageTotal     uint64   `json:"storage_total"`
	StoragePercent   float64  `json:"storage_percent"`
	NetworkInterface string   `json:"network_interface"`
	UploadBPS        float64  `json:"upload_bps"`
	DownloadBPS      float64  `json:"download_bps"`
	UploadedTotal    uint64   `json:"uploaded_total"`
	DownloadedTotal  uint64   `json:"downloaded_total"`
	Sockets          int      `json:"sockets"`
	TCPSockets       int      `json:"tcp_sockets"`
	UDPSockets       int      `json:"udp_sockets"`
	UptimeSeconds    int64    `json:"uptime_seconds"`
	PanelMemory      uint64   `json:"panel_memory"`
	PanelGoroutines  int      `json:"panel_goroutines"`
	ServerIP         string   `json:"server_ip"`
	ServerIPs        []string `json:"server_ips"`
}

func newMetricsCollector() *metricsCollector {
	idle, total, _ := readCPUTimes()
	interfaceName := defaultRouteInterface()
	rx, tx, _ := readInterfaceCounters(interfaceName)
	return &metricsCollector{lastCPUIdle: idle, lastCPUTotal: total, lastRX: rx, lastTX: tx,
		lastNetwork: time.Now(), interfaceName: interfaceName}
}

// CollectMetrics returns one live snapshot for external management tools.
func CollectMetrics() liveMetrics { return newMetricsCollector().collect() }

func (c *metricsCollector) collect() liveMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	result := liveMetrics{Timestamp: now.UnixMilli(), CPUCores: runtime.NumCPU(), PanelGoroutines: runtime.NumGoroutine()}

	idle, total, err := readCPUTimes()
	if err == nil && total > c.lastCPUTotal {
		totalDelta := total - c.lastCPUTotal
		idleDelta := idle - c.lastCPUIdle
		if idleDelta <= totalDelta {
			result.CPUPercent = clampPercent(100 * float64(totalDelta-idleDelta) / float64(totalDelta))
		}
		c.lastCPUIdle, c.lastCPUTotal = idle, total
	}
	result.Load1, result.Load5, result.Load15 = readLoadAverage()

	memory := readMemoryInfo()
	result.MemoryTotal = memory["MemTotal"]
	available := memory["MemAvailable"]
	if result.MemoryTotal >= available {
		result.MemoryUsed = result.MemoryTotal - available
	}
	result.MemoryPercent = percent(result.MemoryUsed, result.MemoryTotal)
	result.SwapTotal = memory["SwapTotal"]
	if result.SwapTotal >= memory["SwapFree"] {
		result.SwapUsed = result.SwapTotal - memory["SwapFree"]
	}
	result.SwapPercent = percent(result.SwapUsed, result.SwapTotal)

	var fs syscall.Statfs_t
	if syscall.Statfs("/", &fs) == nil {
		result.StorageTotal = fs.Blocks * uint64(fs.Bsize)
		availableBytes := fs.Bavail * uint64(fs.Bsize)
		if result.StorageTotal >= availableBytes {
			result.StorageUsed = result.StorageTotal - availableBytes
		}
		result.StoragePercent = percent(result.StorageUsed, result.StorageTotal)
	}

	if c.interfaceName == "" {
		c.interfaceName = defaultRouteInterface()
	}
	result.NetworkInterface = c.interfaceName
	rx, tx, err := readInterfaceCounters(c.interfaceName)
	if err == nil {
		elapsed := now.Sub(c.lastNetwork).Seconds()
		if elapsed > 0 && rx >= c.lastRX && tx >= c.lastTX {
			result.DownloadBPS = float64(rx-c.lastRX) / elapsed
			result.UploadBPS = float64(tx-c.lastTX) / elapsed
		}
		result.DownloadedTotal, result.UploadedTotal = rx, tx
		c.lastRX, c.lastTX, c.lastNetwork = rx, tx, now
	}

	result.Sockets, result.TCPSockets, result.UDPSockets = readSocketStats()
	result.UptimeSeconds = readUptime()
	result.PanelMemory = readProcessRSS()
	result.ServerIPs = interfaceAddresses(c.interfaceName)
	if len(result.ServerIPs) > 0 {
		result.ServerIP = result.ServerIPs[0]
	}
	return result
}

func readCPUTimes() (idle, total uint64, err error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0, 0, errors.New("missing cpu counters")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, errors.New("invalid cpu counters")
	}
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, parseErr := strconv.ParseUint(field, 10, 64)
		if parseErr != nil {
			return 0, 0, parseErr
		}
		values = append(values, value)
		total += value
	}
	idle = values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return idle, total, nil
}

func readMemoryInfo() map[string]uint64 {
	result := map[string]uint64{}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return result
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			result[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	return result
}

func readLoadAverage() (float64, float64, float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	a, _ := strconv.ParseFloat(fields[0], 64)
	b, _ := strconv.ParseFloat(fields[1], 64)
	c, _ := strconv.ParseFloat(fields[2], 64)
	return a, b, c
}

func defaultRouteInterface() string {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[1] == "00000000" {
			return fields[0]
		}
	}
	return ""
}

func readInterfaceCounters(interfaceName string) (rx, tx uint64, err error) {
	if interfaceName == "" {
		return 0, 0, errors.New("no default interface")
	}
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != interfaceName {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 9 {
			return 0, 0, errors.New("invalid network counters")
		}
		rx, err = strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		tx, err = strconv.ParseUint(fields[8], 10, 64)
		return rx, tx, err
	}
	return 0, 0, errors.New("network interface not found")
}

func readSocketStats() (total, tcp, udp int) {
	data, err := os.ReadFile("/proc/net/sockstat")
	if err != nil {
		return 0, 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		switch fields[0] {
		case "sockets:":
			total, _ = strconv.Atoi(fields[2])
		case "TCP:":
			tcp, _ = strconv.Atoi(fields[2])
		case "UDP:":
			udp, _ = strconv.Atoi(fields[2])
		}
	}
	return total, tcp, udp
}

func readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	value, _ := strconv.ParseFloat(fields[0], 64)
	return int64(value)
}

func readProcessRSS() uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, _ := strconv.ParseUint(fields[1], 10, 64)
	return pages * uint64(os.Getpagesize())
}

func interfaceIPv4(interfaceName string) string {
	for _, ip := range interfaceAddresses(interfaceName) {
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			return ip
		}
	}
	return ""
}

func interfaceAddresses(interfaceName string) []string {
	item, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil
	}
	addresses, err := item.Addrs()
	if err != nil {
		return nil
	}
	result := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		result = append(result, ip.String())
	}
	return result
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clampPercent(float64(used) * 100 / float64(total))
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
