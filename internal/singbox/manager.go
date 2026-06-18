package singbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	// DefaultBinaryPath is the default location for the sing-box binary.
	DefaultBinaryPath = "/usr/local/bin/sing-box"
	// DefaultConfigDir is the default directory for sing-box config and certs.
	DefaultConfigDir = "/etc/sing-box"
	// DefaultConfigPath is the default sing-box config file path.
	DefaultConfigPath = "/etc/sing-box/config.json"
	// CertFile is the auto-generated self-signed certificate path.
	CertFile = "/etc/sing-box/server.crt"
	// KeyFile is the auto-generated self-signed key path.
	KeyFile = "/etc/sing-box/server.key"
	// SBBasePort is the starting port for sing-box inbounds (21000-21999).
	SBBasePort = 21000
	// SBMaxPort is the max port for sing-box inbounds.
	SBMaxPort = 21999
)

var execCommand = exec.Command

const (
	primaryServiceName = "sing-box"
	legacyServiceName  = "migate-singbox"
)

// ManagementStatus describes whether sing-box is managed by a known systemd
// unit and which unit runtime operations should use.
type ManagementStatus struct {
	Managed bool
	Service string
}

// IsInstalled returns true if the sing-box binary exists.
func IsInstalled() bool {
	_, err := os.Stat(DefaultBinaryPath)
	return err == nil
}

// CheckConfigDir ensures the config directory exists.
func CheckConfigDir() error {
	return os.MkdirAll(DefaultConfigDir, 0755)
}

// Version returns the sing-box version string.
func Version() (string, error) {
	if !IsInstalled() {
		return "", fmt.Errorf("sing-box not installed")
	}
	out, err := execCommand(DefaultBinaryPath, "version").Output()
	if err != nil {
		return "", fmt.Errorf("sing-box version: %w", err)
	}
	return NormalizeVersion(string(out)), nil
}

// NormalizeVersion keeps the compact user-facing sing-box version line and
// drops verbose build metadata such as "Tags:" and later lines.
func NormalizeVersion(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Tags:") {
			continue
		}
		return line
	}
	return strings.TrimSpace(raw)
}

// CheckConfig validates the config file with sing-box.
func CheckConfig() error {
	return CheckConfigPath(DefaultConfigPath)
}

// CheckConfigPath validates a specific config file with sing-box.
func CheckConfigPath(path string) error {
	out, err := execCommand(DefaultBinaryPath, "check", "-c", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sing-box config check failed: %s: %w", string(out), err)
	}
	return nil
}

// Status returns "running" if the service is active, "stopped" if it is
// managed but inactive, or "not_managed" when no known systemd unit exists.
func Status() string {
	if !Management().Managed {
		return "not_managed"
	}
	out, err := ServiceStatus()
	if err != nil {
		return "stopped"
	}
	status := strings.TrimSpace(string(out))
	switch status {
	case "active", "activating":
		return "running"
	default:
		return "stopped"
	}
}

// Apply writes the config file, checks config validity, and restarts the service.
func Apply() error {
	if err := CheckConfig(); err != nil {
		return fmt.Errorf("config check failed: %w", err)
	}
	_, err := RestartService()
	return err
}

// ApplyConfig atomically validates, installs, and restarts a generated config.
func ApplyConfig(raw []byte) error {
	if err := CheckConfigDir(); err != nil {
		return fmt.Errorf("prepare config dir: %w", err)
	}
	tmp, err := os.CreateTemp(DefaultConfigDir, ".config-*.json")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := CheckConfigPath(tmpPath); err != nil {
		return fmt.Errorf("config check failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(DefaultConfigPath), 0755); err != nil {
		return fmt.Errorf("prepare config path: %w", err)
	}
	var backupPath string
	if _, err := os.Stat(DefaultConfigPath); err == nil {
		backup, createErr := os.CreateTemp(filepath.Dir(DefaultConfigPath), ".config-backup-*.json")
		if createErr != nil {
			return fmt.Errorf("backup current config: %w", createErr)
		}
		backupPath = backup.Name()
		if data, readErr := os.ReadFile(DefaultConfigPath); readErr != nil {
			backup.Close()
			return fmt.Errorf("backup current config: %w", readErr)
		} else if _, writeErr := backup.Write(data); writeErr != nil {
			backup.Close()
			return fmt.Errorf("backup current config: %w", writeErr)
		} else if closeErr := backup.Close(); closeErr != nil {
			return fmt.Errorf("backup current config: %w", closeErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat current config: %w", err)
	}
	if err := os.Rename(tmpPath, DefaultConfigPath); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	if _, err := RestartService(); err != nil {
		if backupPath != "" {
			if restoreErr := os.Rename(backupPath, DefaultConfigPath); restoreErr != nil {
				return fmt.Errorf("%w; restore previous config failed: %v", err, restoreErr)
			}
			if _, restoreRestartErr := RestartService(); restoreRestartErr != nil {
				return fmt.Errorf("%w; previous config restored but restart failed: %v", err, restoreRestartErr)
			}
			return err
		}
		return fmt.Errorf("%w; new config was installed but service did not start because no previous config was available to restore", err)
	}
	if backupPath != "" {
		_ = os.Remove(backupPath)
	}
	return nil
}

// ServiceName returns the systemd service name.
func ServiceName() string {
	return primaryServiceName
}

// LegacyServiceName returns the pre-migration systemd unit name. New installs
// use ServiceName(), but runtime operations keep this fallback for upgraded VPSes.
func LegacyServiceName() string {
	return legacyServiceName
}

// RuntimeServiceName returns the best systemd unit to use for runtime
// operations, preferring the upstream sing-box.service and falling back to the
// legacy MiGate-managed unit when it is the only installed unit.
func RuntimeServiceName() string {
	return resolveServiceName()
}

func Management() ManagementStatus {
	if serviceAvailable(primaryServiceName) {
		return ManagementStatus{Managed: true, Service: primaryServiceName}
	}
	if serviceAvailable(legacyServiceName) {
		return ManagementStatus{Managed: true, Service: legacyServiceName}
	}
	return ManagementStatus{Managed: false, Service: primaryServiceName}
}

func resolveServiceName() string {
	return Management().Service
}

func serviceAvailable(name string) bool {
	cmd := execCommand("systemctl", "show", name, "--property=LoadState", "--value")
	out, err := cmd.CombinedOutput()
	state := strings.TrimSpace(string(out))
	if err != nil {
		return false
	}
	switch state {
	case "loaded", "generated", "linked", "linked-runtime", "masked", "masked-runtime", "static", "indirect", "enabled", "disabled":
		return true
	default:
		return false
	}
}

// RestartService restarts the systemd service.
func RestartService() (string, error) {
	service := resolveServiceName()
	cmd := execCommand("systemctl", "restart", service)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("systemctl restart %s failed: %w", service, err)
	}
	return string(out), nil
}

// ServiceStatus returns the systemd service status.
func ServiceStatus() (string, error) {
	service := resolveServiceName()
	cmd := execCommand("systemctl", "is-active", service)
	out, err := cmd.CombinedOutput()
	if err == nil || strings.TrimSpace(string(out)) != "" {
		return string(out), nil
	}
	return "", fmt.Errorf("systemctl is-active %s failed: %w", service, err)
}

// ConfigPath returns the full path for a given config file name.
func ConfigPath() string {
	return DefaultConfigPath
}

// NextPort finds the next available port for a new sing-box inbound.
// Returns SBBasePort + count, clamped to SBMaxPort.
func NextPort(count int) int {
	port := SBBasePort + count
	if port > SBMaxPort {
		port = SBMaxPort
	}
	return port
}

// ServiceProperties holds parsed systemctl show data for the sing-box service.
type ServiceProperties struct {
	MemoryRSS                     int64
	MainPID                       int64
	ActiveEnterTimestamp          string
	ActiveEnterTimestampMonotonic int64
}

// Show returns parsed systemd service properties via systemctl show.
func Show() (*ServiceProperties, error) {
	service := resolveServiceName()
	cmd := execCommand("systemctl", "show", service,
		"--property=MemoryCurrent",
		"--property=MainPID",
		"--property=ActiveEnterTimestamp",
		"--property=ActiveEnterTimestampMonotonic")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("systemctl show %s: %w", service, err)
	}
	props := &ServiceProperties{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MemoryCurrent=") {
			val := strings.TrimPrefix(line, "MemoryCurrent=")
			props.MemoryRSS, _ = strconv.ParseInt(val, 10, 64)
		} else if strings.HasPrefix(line, "MainPID=") {
			val := strings.TrimPrefix(line, "MainPID=")
			props.MainPID, _ = strconv.ParseInt(val, 10, 64)
		} else if strings.HasPrefix(line, "ActiveEnterTimestamp=") {
			props.ActiveEnterTimestamp = strings.TrimPrefix(line, "ActiveEnterTimestamp=")
		}
	}
	return props, nil
}

// MemoryRSS returns the current RSS memory usage in bytes.
func MemoryRSS() int64 {
	props, err := Show()
	if err != nil {
		return 0
	}
	return props.MemoryRSS
}

// Uptime returns a human-readable uptime string (e.g. "2h15m").
func Uptime() string {
	props, err := Show()
	if err != nil {
		return "未知"
	}
	ts := props.ActiveEnterTimestamp
	if ts == "" {
		return "未知"
	}
	layout := "Mon 2006-01-02 15:04:05 MST"
	t, err := time.Parse(layout, ts)
	if err != nil {
		return "未知"
	}
	dur := time.Since(t)
	if dur < 0 {
		return "刚启动"
	}
	h := int(dur.Hours())
	m := int(dur.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// ActiveConnections returns the number of established TCP connections
// to sing-box ports (21000-21999 range) via ss.
func ActiveConnections() int {
	out, err := execCommand("ss", "-tn", "state", "established").CombinedOutput()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		for port := SBBasePort; port <= SBMaxPort; port++ {
			if strings.Contains(line, fmt.Sprintf(":%d ", port)) ||
				strings.HasSuffix(strings.TrimSpace(line), fmt.Sprintf(":%d", port)) {
				count++
				break
			}
		}
	}
	return count
}

// ListeningUDPPorts reports whether expected UDP ports are currently listening.
func ListeningUDPPorts(expected []int) map[int]bool {
	result := map[int]bool{}
	for _, port := range expected {
		if port > 0 && port <= 65535 {
			result[port] = false
		}
	}
	out, err := execCommand("ss", "-H", "-lun").CombinedOutput()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		port := parseAddressPort(fields[3])
		if _, ok := result[port]; ok {
			result[port] = true
		}
	}
	return result
}

func parseAddressPort(address string) int {
	address = strings.TrimSpace(address)
	if address == "" {
		return 0
	}
	if idx := strings.LastIndex(address, ":"); idx >= 0 && idx < len(address)-1 {
		port, _ := strconv.Atoi(strings.Trim(address[idx+1:], "[]"))
		return port
	}
	return 0
}
