package middleware

import (
	"net/http"
	"regexp"
)

// PathRewrite supports regex matching and replace on Request URIs
func PathRewrite(regexp *regexp.Regexp, replacement string) Interface {
	return pathRewrite{
		regexp:      regexp,
		replacement: replacement,
	}
}

type pathRewrite struct {
	regexp      *regexp.Regexp
	replacement string
}

func (p pathRewrite) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RequestURI = p.regexp.ReplaceAllString(r.RequestURI, p.replacement)
		r.URL.Path = p.regexp.ReplaceAllString(r.URL.Path, p.replacement)
		next.ServeHTTP(w, r)
	})
}

// PathReplace replcase Request.RequestURI with the specified string.
func PathReplace(replacement string) Interface {
	return pathReplace(replacement)
}

type pathReplace string

func (p pathReplace) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = string(p)
		r.RequestURI = string(p)
		next.ServeHTTP(w, r)
	})
}
