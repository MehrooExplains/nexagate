package app

import (
	"bytes"
	"embed"
	"net/http"
	"strings"
	"time"
)

// fontAssets keeps the panel typography available without a CDN or network
// request from the administrator's browser.
//
//go:embed assets/fonts/*.woff2
var fontAssets embed.FS

var allowedFonts = map[string]string{
	"Inter-v4.1.woff2":                   "693b77d4f32ee9b8bfc995589b5fad5e99adf2832738661f5402f9978429a8e3",
	"JetBrainsMono-Bold-v2.304.woff2":    "c503cc5ec5f8b2c7666b7ecda1adf44bd45f2e6579b2eba0fc292150416588a2",
	"JetBrainsMono-Regular-v2.304.woff2": "a9cb1cd82332b23a47e3a1239d25d13c86d16c4220695e34b243effa999f45f2",
	"Vazirmatn-v33.003.woff2":            "4e3fa217d38fdafc1fea4414ceb58ca5e662cf0ab5fa735a8c8c20e8b42cad92",
}

func (s *server) fontAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/assets/fonts/")
	digest, ok := allowedFonts[name]
	if !ok || name == "" {
		http.NotFound(w, r)
		return
	}
	etag := `"` + digest + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	data, err := fontAssets.ReadFile("assets/fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
