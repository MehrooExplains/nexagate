package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type auditEvent struct {
	Time     time.Time `json:"time"`
	Action   string    `json:"action"`
	Target   string    `json:"target"`
	RemoteIP string    `json:"remote_ip"`
}

func (s *server) audit(r *http.Request, action, target string) {
	e := auditEvent{Time: time.Now().UTC(), Action: action, Target: target, RemoteIP: clientIP(r)}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	data = append(data, '\n')
	path := filepath.Join(filepath.Dir(s.configPath), "audit.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}
