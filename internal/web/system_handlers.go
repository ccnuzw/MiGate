package web

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	runtimecmd "github.com/imzyb/MiGate/internal/runtime/command"
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
		writeJSON(w, http.StatusOK, StatusResponse{Status: "restarting"})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Fork a child that restarts after a brief delay so the response is sent first
		go func() {
			time.Sleep(500 * time.Millisecond)
			_ = runtimecmd.Run(context.Background(), "systemctl", "restart", "migate")
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
		status, detail := "unknown", ""
		out, err := runtimecmd.RunOutput(r.Context(), "systemctl", "is-active", "migate")
		if err == nil {
			status = strings.TrimSpace(string(out))
		}
		if status == "active" {
			out2, _ := runtimecmd.RunOutput(r.Context(), "systemctl", "show", "migate", "--property=ActiveEnterTimestamp", "--value")
			if len(out2) > 0 {
				detail = "启动于 " + strings.TrimSpace(string(out2))
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"service": "migate",
			"status":  status,
			"detail":  detail,
		})
	}
}
