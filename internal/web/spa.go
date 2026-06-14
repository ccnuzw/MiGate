package web

import (
	"encoding/json"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/web/static"
)

func staticAssetsHandler() http.Handler {
	return precompressedStaticHandler("/assets/", static.Assets())
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
		nonce := cspNonceFromRequest(r)
		baseJSON, _ := json.Marshal(basePath)
		injected := withScriptNonce(string(index), nonce)
		basePathScript := `<script nonce="` + nonce + `">window.__MIGATE_BASE_PATH__=` + string(baseJSON) + `;</script>`
		injected = strings.Replace(injected, "</head>", basePathScript+"</head>", 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(injected))
	}
}

func withScriptNonce(html, nonce string) string {
	if nonce == "" {
		return html
	}
	return strings.ReplaceAll(html, "<script>", `<script nonce="`+nonce+`">`)
}

func cacheStaticAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cached := &cacheControlWriter{ResponseWriter: w}
		next.ServeHTTP(cached, r)
	})
}

func precompressedStaticHandler(prefix string, filesystem fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, prefix)
		name = path.Clean("/" + name)
		if name == "/" || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		name = strings.TrimPrefix(name, "/")
		encoding := ""
		servedName := name
		for _, candidate := range preferredEncodings(r) {
			compressedName := name + candidate.suffix
			if fileExists(filesystem, compressedName) {
				encoding = candidate.name
				servedName = compressedName
				break
			}
		}
		file, err := filesystem.Open(servedName)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		content, ok := file.(io.ReadSeeker)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("Vary", "Accept-Encoding")
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if encoding != "" {
			w.Header().Set("Content-Encoding", encoding)
		}
		http.ServeContent(w, r, name, stat.ModTime(), content)
	})
}

type encodingCandidate struct {
	name   string
	suffix string
	q      float64
}

func preferredEncodings(r *http.Request) []encodingCandidate {
	candidates := []encodingCandidate{
		{name: "br", suffix: ".br", q: acceptedEncodingQ(r, "br")},
		{name: "gzip", suffix: ".gz", q: acceptedEncodingQ(r, "gzip")},
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if candidate.q > 0 {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 2 && filtered[1].q > filtered[0].q {
		filtered[0], filtered[1] = filtered[1], filtered[0]
	}
	return filtered
}

func acceptedEncodingQ(r *http.Request, encoding string) float64 {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		pieces := strings.Split(part, ";")
		value := strings.TrimSpace(pieces[0])
		if strings.EqualFold(value, encoding) {
			q := 1.0
			for _, param := range pieces[1:] {
				key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
				if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
					parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
					if err != nil {
						return 0
					}
					q = parsed
				}
			}
			if q <= 0 {
				return 0
			}
			return q
		}
	}
	return 0
}

func fileExists(filesystem fs.FS, name string) bool {
	file, err := filesystem.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	stat, err := file.Stat()
	return err == nil && !stat.IsDir()
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
