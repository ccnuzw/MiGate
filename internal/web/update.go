package web

import (
	"net/http"

	updatesvc "github.com/imzyb/MiGate/internal/service/update"
)

const defaultUpdateCheckURL = updatesvc.DefaultCheckURL

func versionHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if version == "" {
			version = "dev"
		}
		writeJSON(w, http.StatusOK, map[string]string{"version": version})
	}
}

func updateCheckHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		result, err := updateService(cfg.version, cfg.updateCheckURL).Check(r.Context(), cfg.version)
		if err != nil {
			writeServiceError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func updateStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, updateService("", "").Status())
	}
}

func updateLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, updateService("", "").Logs(r.URL.Query().Get("lines")))
	}
}

func updateHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		response, status, started, err := updateService(version, "").Start(r.Context(), version)
		if err != nil {
			writeServiceError(w, http.StatusServiceUnavailable, err)
			return
		}
		if !started {
			writeJSON(w, http.StatusConflict, status)
			return
		}
		writeJSON(w, http.StatusOK, response)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func updateService(version, checkURL string) updatesvc.Service {
	_ = version
	return updatesvc.Service{
		CheckURL: checkURL,
		TestMode: runningUnderGoTest(),
		MaxLines: maxXrayLogLines,
	}
}
