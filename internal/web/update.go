package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultUpdateCheckURL = "https://api.github.com/repos/imzyb/MiGate/releases/latest"
const updateLogPath = "/var/log/migate-update.log"

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

func updateLogsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		lines := clampUpdateLogLines(r.URL.Query().Get("lines"))
		logs, err := readUpdateLogs(lines)
		if err != nil {
			logs = fmt.Sprintf("无法读取更新日志：%v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"logs": logs, "path": updateLogPath})
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
		current := strings.TrimSpace(version)
		if current == "" {
			current = "dev"
		}
		if !runningUnderGoTest() {
			if err := validateUpdaterAvailable(); err != nil {
				writeJSONError(w, http.StatusServiceUnavailable, "updater_unavailable", map[string]interface{}{"detail": err.Error()})
				return
			}
		}
		status, started := globalUpdateState.start(current)
		if !started {
			writeJSON(w, http.StatusConflict, status)
			return
		}
		command := "/usr/local/bin/migate-install --update --yes"
		_ = appendUpdateLog(fmt.Sprintf("\n[%s] WebUI requested MiGate update from %s\n", time.Now().Format(time.RFC3339), current))
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
			unit := fmt.Sprintf("migate-update-%d-%d", os.Getpid(), time.Now().UnixNano())
			cmd := exec.Command("systemd-run", "--wait", "--unit="+unit, "--property=Type=oneshot", "--property=User=root", "--property=TimeoutSec=300", "--property=StandardOutput=append:/var/log/migate-update.log", "--property=StandardError=append:/var/log/migate-update.log", "/usr/local/bin/migate-install", "--update", "--yes")
			out, err := cmd.CombinedOutput()
			if len(out) > 0 {
				_ = appendUpdateLog(string(out))
			}
			if err != nil {
				message := strings.TrimSpace(string(out))
				if recent, logErr := readUpdateLogs("20"); logErr == nil && strings.TrimSpace(recent) != "" {
					message = strings.TrimSpace(recent)
				}
				if message == "" {
					message = err.Error()
				} else {
					message = err.Error() + ": " + lastNonEmptyLine(message)
				}
				_ = appendUpdateLog(fmt.Sprintf("[%s] update failed: %s\n", time.Now().Format(time.RFC3339), message))
				globalUpdateState.finish("failed", message)
				return
			}
			globalUpdateState.finish("completed", "update command completed; MiGate may restart if a new version was installed")
		}()
	}
}

func validateUpdaterAvailable() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("MiGate service must run as root to start the updater")
	}
	if _, err := exec.LookPath("systemd-run"); err != nil {
		return fmt.Errorf("systemd-run not found: %w", err)
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return fmt.Errorf("systemd is not available: %w", err)
	}
	if info, err := os.Stat("/usr/local/bin/migate-install"); err != nil {
		return fmt.Errorf("/usr/local/bin/migate-install not available: %w", err)
	} else if info.IsDir() || info.Mode()&0111 == 0 {
		return fmt.Errorf("/usr/local/bin/migate-install is not executable")
	}
	return nil
}

func readUpdateLogs(lines string) (string, error) {
	if _, err := os.Stat(updateLogPath); err == nil {
		out, err := exec.Command("tail", "-n", lines, updateLogPath).CombinedOutput()
		if err == nil {
			return string(out), nil
		}
		if journalOut, journalErr := readUpdateJournalLogs(lines); journalErr == nil {
			return journalOut, nil
		}
		return string(out), err
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return readUpdateJournalLogs(lines)
}

func readUpdateJournalLogs(lines string) (string, error) {
	if _, err := exec.LookPath("journalctl"); err == nil {
		out, journalErr := exec.Command("journalctl", "-u", "migate-update", "-u", "migate-update-*", "-n", lines, "--no-pager", "-o", "short-iso").CombinedOutput()
		if journalErr == nil {
			return string(out), nil
		}
		return string(out), journalErr
	}
	return "", fmt.Errorf("%s 不存在，且 journalctl 不可用", updateLogPath)
}

func appendUpdateLog(entry string) error {
	f, err := os.OpenFile(updateLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

func clampUpdateLogLines(value string) string {
	if value == "" {
		return "120"
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return "120"
	}
	if n > maxXrayLogLines {
		return strconv.Itoa(maxXrayLogLines)
	}
	return strconv.Itoa(n)
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
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
