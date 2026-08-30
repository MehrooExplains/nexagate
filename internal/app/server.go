package app

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type server struct {
	cfg        Config
	cfgMu      sync.RWMutex
	configPath string
	store      *Store
	sessionKey []byte
	templates  *template.Template
	version    string
	metrics    *metricsCollector
	loginMu    sync.Mutex
	logins     map[string][]time.Time
}

type pageData struct {
	Title, CSRF, Error, Message, Version  string
	Lang, CurrentPath                     string
	ActiveNav                             string
	PsiphonRegion, PsiphonMode            string
	Users                                 []userView
	Services                              []serviceView
	User                                  *userView
	Config                                Config
	UserCount, ActiveCount, InactiveCount int
	Update                                updateStatus
	UpdatesEnabled                        bool
}

type userView struct {
	User
	Status, StatusClass, Expiry, Created, Initial string
	Links                                         Links
}

type serviceView struct {
	Name, Label, Status string
	Healthy             bool
}

func Serve(configPath, version string) error {
	cfg, migrated, err := loadConfigAndMigrate(configPath)
	if err != nil {
		return err
	}
	// A binary-only one-click upgrade can add new generated settings safely: the
	// existing systemd path unit validates and applies them after this migration.
	if migrated {
		if err := Render(cfg); err != nil {
			return fmt.Errorf("render migrated configuration: %w", err)
		}
		if err := saveJSONAtomic(configPath, cfg, 0600); err != nil {
			return fmt.Errorf("save migrated configuration: %w", err)
		}
	}
	key, err := base64.RawStdEncoding.DecodeString(cfg.SessionKey)
	if err != nil || len(key) != 32 {
		return errors.New("invalid session key")
	}
	templates, err := parsePageTemplates()
	if err != nil {
		return err
	}
	s := &server{cfg: cfg, configPath: configPath, store: NewStore(cfg.StatePath), sessionKey: key, templates: templates,
		version: version, metrics: newMetricsCollector(), logins: map[string][]time.Time{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/assets/fonts/", s.fontAsset)
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/language", s.language)
	mux.HandleFunc("/login", s.login)
	mux.HandleFunc("/logout", s.auth(s.logout))
	mux.HandleFunc("/users", s.auth(s.users))
	mux.HandleFunc("/", s.auth(s.dashboard))
	mux.HandleFunc("/users/add", s.auth(s.requireCSRF(s.addUser)))
	mux.HandleFunc("/users/toggle", s.auth(s.requireCSRF(s.toggleUser)))
	mux.HandleFunc("/users/delete", s.auth(s.requireCSRF(s.deleteUser)))
	mux.HandleFunc("/users/links", s.auth(s.links))
	mux.HandleFunc("/users/qr", s.auth(s.qr))
	mux.HandleFunc("/settings/inbounds", s.auth(s.inbounds))
	mux.HandleFunc("/settings/outbounds", s.auth(s.outbounds))
	mux.HandleFunc("/settings/routing", s.auth(s.routing))
	mux.HandleFunc("/settings/panel", s.auth(s.panelSettings))
	mux.HandleFunc("/settings/psiphon", s.auth(s.requireCSRF(s.updatePsiphon)))
	mux.HandleFunc("/api/metrics", s.auth(s.metricsAPI))
	mux.HandleFunc("/api/users", s.auth(s.usersAPI))
	mux.HandleFunc("/metrics", s.auth(s.prometheusMetrics))
	mux.HandleFunc("/api/update-status", s.auth(s.updateStatusAPI))
	mux.HandleFunc("/updates/run", s.auth(s.requireCSRF(s.triggerUpdate)))
	handler := s.securityHeaders(mux)
	httpServer := &http.Server{Addr: cfg.Listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Printf("NexaGate %s listening on %s", version, cfg.Listen)
	return httpServer.ListenAndServe()
}

func (s *server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "ok\n")
}

func (s *server) metricsAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(s.metrics.collect())
}

func (s *server) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	m := s.metrics.collect()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	f := func(name string, value any) { _, _ = fmt.Fprintf(w, "nexagate_%s %v\n", name, value) }
	f("cpu_percent", m.CPUPercent)
	f("memory_used_bytes", m.MemoryUsed)
	f("memory_total_bytes", m.MemoryTotal)
	f("swap_used_bytes", m.SwapUsed)
	f("storage_used_bytes", m.StorageUsed)
	f("upload_bytes_per_second", m.UploadBPS)
	f("download_bytes_per_second", m.DownloadBPS)
	f("uploaded_bytes_total", m.UploadedTotal)
	f("downloaded_bytes_total", m.DownloadedTotal)
	f("open_sockets", m.Sockets)
	f("tcp_sockets", m.TCPSockets)
	f("udp_sockets", m.UDPSockets)
	f("uptime_seconds", m.UptimeSeconds)
}

func (s *server) usersAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		db, err := s.store.Load()
		if err != nil {
			http.Error(w, "internal error", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(db.Users)
	case http.MethodPost:
		var in struct {
			Name string `json:"name"`
			Days int    `json:"days"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		user, err := s.store.Add(in.Name, in.Days)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	lang := languageFromRequest(r)
	if r.Method == http.MethodGet {
		s.render(w, r, "login", pageData{Title: localize(lang, "ورود | NexaGate", "Login | NexaGate")})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.allowLogin(clientIP(r)) {
		s.renderStatus(w, r, "login", pageData{Title: localize(lang, "ورود | NexaGate", "Login | NexaGate"), Error: localize(lang, "تعداد تلاش‌ها بیش از حد است؛ کمی بعد دوباره امتحان کنید.", "Too many attempts. Try again later.")}, http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil || !verifyPassword(s.cfg.AdminHash, r.FormValue("password")) {
		time.Sleep(350 * time.Millisecond)
		s.renderStatus(w, r, "login", pageData{Title: localize(lang, "ورود | NexaGate", "Login | NexaGate"), Error: localize(lang, "رمز عبور نادرست است.", "Invalid password.")}, http.StatusUnauthorized)
		return
	}
	nonce, err := randomToken(24)
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "nexagate_session", Value: signSession(s.sessionKey, time.Now().Add(12*time.Hour), nonce), Path: "/", MaxAge: 43200, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) language(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "fa" && lang != "en" {
		lang = "fa"
	}
	http.SetCookie(w, &http.Cookie{
		Name: languageCookie, Value: lang, Path: "/", MaxAge: 365 * 24 * 60 * 60,
		HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, safeLocalRedirect(r.URL.Query().Get("next")), http.StatusSeeOther)
}

func (s *server) allowLogin(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	now := time.Now()
	cutoff := now.Add(-15 * time.Minute)
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	items := s.logins[host][:0]
	for _, item := range s.logins[host] {
		if item.After(cutoff) {
			items = append(items, item)
		}
	}
	if len(items) >= 8 {
		s.logins[host] = items
		return false
	}
	s.logins[host] = append(items, now)
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remoteIP := net.ParseIP(host)
	if remoteIP != nil && remoteIP.IsLoopback() {
		if forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); forwarded != nil {
			return forwarded.String()
		}
	}
	return host
}

func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("nexagate_session")
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		nonce, ok := verifySession(s.sessionKey, cookie.Value, time.Now())
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		r.Header.Set("X-NexaGate-CSRF", csrfToken(s.sessionKey, nonce))
		next(w, r)
	}
}

func (s *server) requireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil || subtle.ConstantTimeCompare([]byte(r.FormValue("csrf")), []byte(r.Header.Get("X-NexaGate-CSRF"))) != 1 {
			http.Error(w, "invalid request token", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "nexagate_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	lang := languageFromRequest(r)
	users, total, active, inactive, err := s.userViews(lang)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	_ = users
	s.render(w, r, "dashboard", pageData{
		Title: localize(lang, "نمای کلی | NexaGate", "Overview | NexaGate"), ActiveNav: "overview", CSRF: r.Header.Get("X-NexaGate-CSRF"),
		Services: s.serviceViews(lang), UserCount: total, ActiveCount: active, InactiveCount: inactive,
		Update: s.readUpdateStatus(), UpdatesEnabled: s.version != "dev",
	})
}

func (s *server) users(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/users" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	lang := languageFromRequest(r)
	users, total, active, inactive, err := s.userViews(lang)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	s.render(w, r, "users", pageData{
		Title: localize(lang, "کاربران | NexaGate", "Users | NexaGate"), ActiveNav: "users", CSRF: r.Header.Get("X-NexaGate-CSRF"),
		Users: users, UserCount: total, ActiveCount: active, InactiveCount: inactive,
	})
}

func (s *server) userViews(lang string) ([]userView, int, int, int, error) {
	db, err := s.store.Load()
	if err != nil {
		return nil, 0, 0, 0, err
	}
	now := time.Now().UTC()
	views := make([]userView, 0, len(db.Users))
	active := 0
	for _, user := range db.Users {
		status := localize(lang, "فعال", "Active")
		statusClass := "active"
		if !user.Enabled {
			status = localize(lang, "غیرفعال", "Disabled")
			statusClass = ""
		} else if !user.ExpiresAt.IsZero() && !user.ExpiresAt.After(now) {
			status = localize(lang, "منقضی", "Expired")
			statusClass = "warn"
		} else {
			active++
		}
		expiry := localize(lang, "بدون انقضا", "Never")
		if !user.ExpiresAt.IsZero() {
			expiry = user.ExpiresAt.Format("2006-01-02")
		}
		initial := "U"
		if user.Name != "" {
			initial = strings.ToUpper(user.Name[:1])
		}
		views = append(views, userView{User: user, Status: status, StatusClass: statusClass, Expiry: expiry,
			Created: user.CreatedAt.Format("2006-01-02"), Initial: initial})
	}
	return views, len(views), active, len(views) - active, nil
}

func (s *server) serviceViews(lang string) []serviceView {
	services := []serviceView{}
	for _, item := range []struct{ name, fa, en string }{{"nexagate-hysteria", "Hysteria2", "Hysteria2"}, {"nexagate-xray", "Xray", "Xray"}, {"nexagate-psiphon", "Psiphon", "Psiphon"}, {"wg-quick@warp0", "WARP", "WARP"}, {"nexagate-dns", "سامانه نام دامنه از طریق WARP", "DNS via WARP"}} {
		ok := serviceActive(item.name)
		state := localize(lang, "متوقف", "Stopped")
		if ok {
			state = localize(lang, "فعال", "Running")
		}
		services = append(services, serviceView{Name: item.name, Label: localize(lang, item.fa, item.en), Status: state, Healthy: ok})
	}
	return services
}

func (s *server) configSnapshot() Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

func (s *server) inbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lang := languageFromRequest(r)
	s.render(w, r, "inbounds", pageData{Title: localize(lang, "ورودی‌ها | NexaGate", "Inbounds | NexaGate"), ActiveNav: "inbounds", Config: s.configSnapshot()})
}

func (s *server) outbounds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := s.configSnapshot()
	lang := languageFromRequest(r)
	mode := localize(lang, "خودکار", "Automatic")
	if cfg.Psiphon.Mode == "fixed" {
		mode = localize(lang, "کشور ثابت", "Fixed region")
	}
	s.render(w, r, "outbounds", pageData{Title: localize(lang, "خروجی‌ها | NexaGate", "Outbounds | NexaGate"), ActiveNav: "outbounds",
		CSRF: r.Header.Get("X-NexaGate-CSRF"), Config: cfg, PsiphonRegion: cfg.Psiphon.Region, PsiphonMode: mode})
}

func (s *server) routing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lang := languageFromRequest(r)
	s.render(w, r, "routing", pageData{Title: localize(lang, "مسیریابی | NexaGate", "Routing | NexaGate"), ActiveNav: "routing", Config: s.configSnapshot()})
}

func (s *server) panelSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lang := languageFromRequest(r)
	s.render(w, r, "panel-settings", pageData{Title: localize(lang, "تنظیمات پنل | NexaGate", "Panel settings | NexaGate"), ActiveNav: "panel", Config: s.configSnapshot()})
}

func (s *server) addUser(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.FormValue("days"))
	if days < 0 || days > 3650 {
		http.Error(w, "invalid expiry", 400)
		return
	}
	if _, err := s.store.Add(strings.TrimSpace(r.FormValue("name")), days); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.renderCurrentConfig(); err != nil {
		http.Error(w, "user saved but configuration rendering failed: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}
func (s *server) toggleUser(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Toggle(r.FormValue("id")); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if err := s.renderCurrentConfig(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}
func (s *server) deleteUser(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Delete(r.FormValue("id")); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if err := s.renderCurrentConfig(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/users", http.StatusSeeOther)
}

func (s *server) updatePsiphon(w http.ResponseWriter, r *http.Request) {
	region := strings.ToUpper(strings.TrimSpace(r.FormValue("region")))
	if region != "" && (len(region) != 2 || region[0] < 'A' || region[0] > 'Z' || region[1] < 'A' || region[1] > 'Z') {
		http.Error(w, "region must be blank (automatic) or a two-letter ISO code", http.StatusBadRequest)
		return
	}
	s.cfgMu.Lock()
	cfg := s.cfg
	cfg.Psiphon.Region = region
	if region == "" {
		cfg.Psiphon.Mode = "auto"
	} else {
		cfg.Psiphon.Mode = "fixed"
	}
	if err := saveJSONAtomic(s.configPath, cfg, 0600); err != nil {
		s.cfgMu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := Render(cfg); err != nil {
		s.cfgMu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg = cfg
	s.cfgMu.Unlock()
	http.Redirect(w, r, "/settings/outbounds", http.StatusSeeOther)
}

func (s *server) renderCurrentConfig() error {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return Render(s.cfg)
}

func (s *server) links(w http.ResponseWriter, r *http.Request) {
	db, err := s.store.Load()
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	var found *User
	for i := range db.Users {
		if db.Users[i].ID == r.URL.Query().Get("id") {
			found = &db.Users[i]
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()
	view := userView{User: *found, Links: userLinks(cfg, *found)}
	lang := languageFromRequest(r)
	s.render(w, r, "links", pageData{Title: localize(lang, "پروفایل‌های اتصال | NexaGate", "Connection profiles | NexaGate"), ActiveNav: "users",
		CSRF: r.Header.Get("X-NexaGate-CSRF"), User: &view})
}

func (s *server) qr(w http.ResponseWriter, r *http.Request) {
	db, err := s.store.Load()
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	var found *User
	for i := range db.Users {
		if db.Users[i].ID == r.URL.Query().Get("id") {
			found = &db.Users[i]
			break
		}
	}
	if found == nil {
		http.NotFound(w, r)
		return
	}
	s.cfgMu.RLock()
	cfg := s.cfg
	s.cfgMu.RUnlock()
	links := userLinks(cfg, *found)
	value := ""
	switch r.URL.Query().Get("type") {
	case "hysteria":
		value = links.Hysteria
	case "xhttp":
		value = links.XHTTPReality
	case "xhttp-tls":
		value = links.XHTTPTLS
	case "reality":
		value = links.RawReality
	case "ws":
		value = links.WebSocketTLS
	default:
		http.Error(w, "invalid profile", 400)
		return
	}
	cmd := exec.Command("/usr/bin/qrencode", "-t", "PNG", "-o", "-", "-m", "2", "-s", "6")
	cmd.Stdin = strings.NewReader(value)
	png, err := cmd.Output()
	if err != nil {
		http.Error(w, "QR generation failed", 500)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

func (s *server) render(w http.ResponseWriter, r *http.Request, name string, data pageData) {
	s.renderStatus(w, r, name, data, 200)
}
func (s *server) renderStatus(w http.ResponseWriter, r *http.Request, name string, data pageData, status int) {
	data.Version = s.version
	data.Lang = languageFromRequest(r)
	data.CurrentPath = r.URL.RequestURI()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template: %v", err)
	}
}

func systemctlStatus(name string) string {
	output, _ := exec.Command("systemctl", "is-active", name).Output()
	return strings.TrimSpace(string(output))
}
func writeText(w http.ResponseWriter, value string) { _, _ = fmt.Fprint(w, value) }
