package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPercentIsBounded(t *testing.T) {
	for _, test := range []struct {
		used, total uint64
		want        float64
	}{{0, 0, 0}, {25, 100, 25}, {110, 100, 100}} {
		if got := percent(test.used, test.total); got != test.want {
			t.Fatalf("percent(%d, %d) = %v, want %v", test.used, test.total, got, test.want)
		}
	}
}

func TestLinuxMetricsCollector(t *testing.T) {
	collector := newMetricsCollector()
	time.Sleep(10 * time.Millisecond)
	metrics := collector.collect()
	if metrics.CPUCores < 1 || metrics.MemoryTotal == 0 || metrics.StorageTotal == 0 {
		t.Fatalf("incomplete host metrics: %+v", metrics)
	}
	for name, value := range map[string]float64{
		"cpu": metrics.CPUPercent, "memory": metrics.MemoryPercent,
		"swap": metrics.SwapPercent, "storage": metrics.StoragePercent,
	} {
		if value < 0 || value > 100 {
			t.Errorf("%s percentage is out of range: %v", name, value)
		}
	}
}

func TestTriggerUpdateQueuesAtomicRequest(t *testing.T) {
	dir := t.TempDir()
	s := &server{cfg: Config{StatePath: filepath.Join(dir, "users.json")}, version: "1.2.3"}
	request := httptest.NewRequest(http.MethodPost, "/updates/run", nil)
	response := httptest.NewRecorder()
	s.triggerUpdate(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	for _, name := range []string{"update-status.json", "update.request"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.Size() == 0 {
			t.Fatalf("%s was not written atomically: %v", name, err)
		}
	}
	if status := s.readUpdateStatus(); status.State != "queued" || status.CurrentVersion != "1.2.3" {
		t.Fatalf("unexpected update status: %+v", status)
	}
}
