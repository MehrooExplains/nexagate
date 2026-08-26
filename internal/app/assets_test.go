package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddedFontDigests(t *testing.T) {
	for name, want := range allowedFonts {
		data, err := fontAssets.ReadFile("assets/fonts/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Errorf("%s digest = %s, want %s", name, got, want)
		}
	}
}

func TestFontAssetResponseAndCaching(t *testing.T) {
	const name = "Inter-v4.1.woff2"
	s := &server{}
	request := httptest.NewRequest(http.MethodGet, "/assets/fonts/"+name, nil)
	response := httptest.NewRecorder()
	s.fontAsset(response, request)
	if response.Code != http.StatusOK || response.Body.Len() == 0 {
		t.Fatalf("font response: status=%d size=%d", response.Code, response.Body.Len())
	}
	if got := response.Header().Get("Content-Type"); got != "font/woff2" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	etag := response.Header().Get("ETag")
	request = httptest.NewRequest(http.MethodGet, "/assets/fonts/"+name, nil)
	request.Header.Set("If-None-Match", etag)
	response = httptest.NewRecorder()
	s.fontAsset(response, request)
	if response.Code != http.StatusNotModified || response.Header().Get("ETag") != etag {
		t.Fatalf("conditional response: status=%d etag=%q", response.Code, response.Header().Get("ETag"))
	}
}

func TestFontAssetRejectsUnknownPaths(t *testing.T) {
	s := &server{}
	response := httptest.NewRecorder()
	s.fontAsset(response, httptest.NewRequest(http.MethodGet, "/assets/fonts/../secret", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
