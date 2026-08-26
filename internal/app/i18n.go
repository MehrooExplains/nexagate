package app

import (
	"html/template"
	"net/http"
	"strings"
)

const languageCookie = "nexagate_lang"

func localize(lang, fa, en string) string {
	if lang == "en" {
		return en
	}
	return fa
}

func languageFromRequest(r *http.Request) string {
	if r != nil {
		if lang := r.URL.Query().Get("lang"); lang == "fa" || lang == "en" {
			return lang
		}
		if cookie, err := r.Cookie(languageCookie); err == nil && (cookie.Value == "fa" || cookie.Value == "en") {
			return cookie.Value
		}
	}
	return "fa"
}

func templateFunctions() template.FuncMap {
	return template.FuncMap{
		"tr": localize,
	}
}

func parsePageTemplates() (*template.Template, error) {
	return template.New("pages").Funcs(templateFunctions()).Parse(pageTemplates)
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	return strings.EqualFold(proto, "https")
}

func safeLocalRedirect(value string) string {
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") || strings.ContainsAny(value, "\r\n") {
		return "/"
	}
	return value
}
