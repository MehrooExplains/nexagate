package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLanguageFromRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := languageFromRequest(request); got != "fa" {
		t.Fatalf("default language = %q, want fa", got)
	}
	request.AddCookie(&http.Cookie{Name: languageCookie, Value: "en"})
	if got := languageFromRequest(request); got != "en" {
		t.Fatalf("cookie language = %q, want en", got)
	}
	request = httptest.NewRequest(http.MethodGet, "/?lang=fa", nil)
	request.AddCookie(&http.Cookie{Name: languageCookie, Value: "en"})
	if got := languageFromRequest(request); got != "fa" {
		t.Fatalf("query language = %q, want fa", got)
	}
}

func TestLanguageHandlerSetsCookieAndRejectsExternalRedirect(t *testing.T) {
	s := &server{}
	request := httptest.NewRequest(http.MethodGet, "/language?lang=en&next=//example.com", nil)
	recorder := httptest.NewRecorder()
	s.language(recorder, request)
	response := recorder.Result()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/" {
		t.Fatalf("status/location = %d %q", response.StatusCode, response.Header.Get("Location"))
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || cookies[0].Name != languageCookie || cookies[0].Value != "en" {
		t.Fatalf("unexpected cookies: %#v", cookies)
	}
}

func TestSafeLocalRedirect(t *testing.T) {
	for _, unsafe := range []string{"", "https://example.com", "//example.com", "/\\example.com", "/page\nLocation: https://example.com"} {
		if got := safeLocalRedirect(unsafe); got != "/" {
			t.Errorf("safeLocalRedirect(%q) = %q, want /", unsafe, got)
		}
	}
	if got := safeLocalRedirect("/users?filter=active"); got != "/users?filter=active" {
		t.Fatalf("safe path changed to %q", got)
	}
}
