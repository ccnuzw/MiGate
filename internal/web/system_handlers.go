package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func runningUnderGoTest() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func restartHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		if _, ok := decodeCoreActionPayload(w, r); !ok {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"restarting"}`))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Fork a child that restarts after a brief delay so the response is sent first
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = exec.Command("systemctl", "restart", "migate").Run()
		}()
		if !runningUnderGoTest() {
			go func() {
				time.Sleep(2 * time.Second)
				os.Exit(0)
			}()
		}
	}
}

func serviceStatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		status, detail := "unknown", ""
		out, err := exec.Command("systemctl", "is-active", "migate").Output()
		if err == nil {
			status = strings.TrimSpace(string(out))
		}
		if status == "active" {
			out2, _ := exec.Command("systemctl", "show", "migate", "--property=ActiveEnterTimestamp", "--value").Output()
			if len(out2) > 0 {
				detail = "启动于 " + strings.TrimSpace(string(out2))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "migate",
			"status":  status,
			"detail":  detail,
		})
	}
}
