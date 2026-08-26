package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type updateStatus struct {
	State          string    `json:"state"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version,omitempty"`
	Message        string    `json:"message"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type updateRequest struct {
	CurrentVersion string    `json:"current_version"`
	RequestedAt    time.Time `json:"requested_at"`
}

func (s *server) updateStatePath(name string) string {
	s.cfgMu.RLock()
	statePath := s.cfg.StatePath
	s.cfgMu.RUnlock()
	return filepath.Join(filepath.Dir(statePath), name)
}

func (s *server) readUpdateStatus() updateStatus {
	status := updateStatus{State: "idle", CurrentVersion: s.version, Message: "Ready to check for updates"}
	data, err := os.ReadFile(s.updateStatePath("update-status.json"))
	if err == nil && json.Unmarshal(data, &status) == nil {
		if status.CurrentVersion == "" {
			status.CurrentVersion = s.version
		}
		return status
	}
	return status
}

func (s *server) updateStatusAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.readUpdateStatus())
}

func (s *server) triggerUpdate(w http.ResponseWriter, r *http.Request) {
	if s.version == "dev" {
		http.Error(w, "updates are unavailable for development builds", http.StatusBadRequest)
		return
	}
	status := s.readUpdateStatus()
	if status.State == "checking" || status.State == "downloading" || status.State == "installing" {
		http.Error(w, "an update is already running", http.StatusConflict)
		return
	}
	now := time.Now().UTC()
	status = updateStatus{State: "queued", CurrentVersion: s.version, Message: "Update requested", UpdatedAt: now}
	if err := saveJSONAtomic(s.updateStatePath("update-status.json"), status, 0644); err != nil {
		http.Error(w, "could not queue update", http.StatusInternalServerError)
		return
	}
	request := updateRequest{CurrentVersion: s.version, RequestedAt: now}
	if err := saveJSONAtomic(s.updateStatePath("update.request"), request, 0600); err != nil {
		if errors.Is(err, os.ErrPermission) {
			http.Error(w, "update directory is not writable", http.StatusInternalServerError)
			return
		}
		http.Error(w, "could not create update request", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/?update=queued", http.StatusSeeOther)
}
