package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/imzyb/MiGate/internal/web/static"
)

func staticAssetsHandler() http.Handler {
	return cacheStaticAssets(http.StripPrefix("/assets/", http.FileServer(http.FS(static.Assets()))))
}

func loginPageHandler(cfg *routerConfig) http.HandlerFunc {
	spa := spaHandler(cfg.basePath)
	login := loginHandler(cfg)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			login(w, r)
			return
		}
		spa(w, r)
	}
}

func spaHandler(basePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/sub/") || strings.HasPrefix(r.URL.Path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w)
			return
		}
		index, err := static.ReadIndex()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "web_assets_not_built")
			return
		}
		baseJSON, _ := json.Marshal(basePath)
		injected := strings.Replace(string(index), "</head>", `<script>window.__MIGATE_BASE_PATH__=`+string(baseJSON)+`;</script></head>`, 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(injected))
	}
}

func cacheStaticAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cached := &cacheControlWriter{ResponseWriter: w}
		next.ServeHTTP(cached, r)
	})
}

type cacheControlWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (w *cacheControlWriter) WriteHeader(status int) {
	if !w.wroteHeader && shouldCacheStaticStatus(status) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *cacheControlWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(body)
}

func shouldCacheStaticStatus(status int) bool {
	return status == http.StatusNotModified || (status >= 200 && status < 300)
}
