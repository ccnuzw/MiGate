package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const defaultUpdateCheckURL = "https://api.github.com/repos/imzyb/MiGate/releases/latest"

type updateRuntimeStatus struct {
	Status         string    `json:"status"`
	CurrentVersion string    `json:"current_version,omitempty"`
	TargetVersion  string    `json:"target_version,omitempty"`
	Message        string    `json:"message,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type updateRuntimeState struct {
	mu      sync.Mutex
	running bool
	status  updateRuntimeStatus
}

var globalUpdateState = &updateRuntimeState{status: updateRuntimeStatus{Status: "idle", Message: "idle"}}

func versionHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if version == "" {
			version = "dev"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"version": version})
	}
}

func updateCheckHandler(cfg *routerConfig) http.HandlerFunc {
	type releaseResponse struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Name    string `json:"name"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		current := strings.TrimSpace(cfg.version)
		if current == "" {
			current = "dev"
		}
		result := map[string]interface{}{
			"current_version":  current,
			"latest_version":   "",
			"update_available": false,
			"release_url":      "",
			"status":           "unknown",
		}
		if current == "dev" {
			result["status"] = "dev"
			result["message"] = "dev builds cannot be checked against releases"
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		checkURL := strings.TrimSpace(cfg.updateCheckURL)
		if checkURL == "" {
			checkURL = defaultUpdateCheckURL
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "update_check_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "MiGate-update-check")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "update_check_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeJSONError(w, http.StatusBadGateway, "update_check_failed", map[string]interface{}{"detail": resp.Status})
			return
		}
		var release releaseResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
			writeJSONError(w, http.StatusBadGateway, "update_check_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		latest := strings.TrimSpace(release.TagName)
		result["latest_version"] = latest
		result["release_url"] = strings.TrimSpace(release.HTMLURL)
		result["release_name"] = strings.TrimSpace(release.Name)
		result["status"] = "ok"
		result["update_available"] = latest != "" && normalizeMiGateVersion(latest) != normalizeMiGateVersion(current)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func updateStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		status := globalUpdateState.snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	}
}

func updateHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		current := strings.TrimSpace(version)
		if current == "" {
			current = "dev"
		}
		status, started := globalUpdateState.start(current)
		if !started {
			writeJSON(w, http.StatusConflict, status)
			return
		}
		command := "/usr/local/bin/migate-install --update"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "updating", "command": command, "message": status.Message})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if runningUnderGoTest() {
			globalUpdateState.finish("started", "update command accepted in test mode")
			return
		}
		go func() {
			time.Sleep(500 * time.Millisecond)
			err := exec.Command("systemd-run", "--wait", "--unit=migate-update", "--replace", "--collect", "--same-dir", "--property=Type=oneshot", "--property=User=root", "--property=TimeoutSec=180", "--property=StandardOutput=append:/var/log/migate-update.log", "--property=StandardError=append:/var/log/migate-update.log", "/usr/local/bin/migate-install", "--update").Run()
			if err != nil {
				globalUpdateState.finish("failed", err.Error())
				return
			}
			globalUpdateState.finish("restarting", "update command started, MiGate will restart shortly")
		}()
	}
}

func normalizeMiGateVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "MiGate version:")
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	return version
}

func (s *updateRuntimeState) start(current string) (updateRuntimeStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return s.status, false
	}
	now := time.Now().UTC()
	s.running = true
	s.status = updateRuntimeStatus{
		Status:         "updating",
		CurrentVersion: current,
		Message:        "update command accepted",
		StartedAt:      now,
		UpdatedAt:      now,
	}
	return s.status, true
}

func (s *updateRuntimeState) finish(status, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.status.Status = status
	s.status.Message = message
	s.status.UpdatedAt = time.Now().UTC()
}

func (s *updateRuntimeState) snapshot() updateRuntimeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}
