package singbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

const StatsUnsupportedMessage = "sing-box binary does not include with_v2ray_api"

type Capability struct {
	V2RayAPIStats bool
	Checked       bool
	Unsupported   bool
	Message       string
}

type CapabilityDetector struct {
	BinaryPath string
	runCheck   func(ctx context.Context, binaryPath, configPath string) ([]byte, error)
}

func DetectCapability(ctx context.Context) Capability {
	return NewCapabilityDetector(DefaultBinaryPath).Detect(ctx)
}

func NewCapabilityDetector(binaryPath string) *CapabilityDetector {
	return &CapabilityDetector{BinaryPath: binaryPath}
}

func (d *CapabilityDetector) Detect(ctx context.Context) Capability {
	binaryPath := strings.TrimSpace(d.BinaryPath)
	if binaryPath == "" {
		binaryPath = DefaultBinaryPath
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	tmp, err := os.CreateTemp("", "migate-sing-box-v2ray-api-*.json")
	if err != nil {
		return Capability{Checked: true, Message: fmt.Sprintf("create temp config: %v", err)}
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	listen, err := localV2RayAPICheckListen()
	if err != nil {
		_ = tmp.Close()
		return Capability{Checked: true, Message: fmt.Sprintf("allocate temp v2ray api listen: %v", err)}
	}
	raw, err := json.Marshal(minimalV2RayAPICheckConfig(listen))
	if err != nil {
		_ = tmp.Close()
		return Capability{Checked: true, Message: fmt.Sprintf("marshal temp config: %v", err)}
	}
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return Capability{Checked: true, Message: fmt.Sprintf("write temp config: %v", err)}
	}
	if err := tmp.Close(); err != nil {
		return Capability{Checked: true, Message: fmt.Sprintf("close temp config: %v", err)}
	}

	runCheck := d.runCheck
	if runCheck == nil {
		runCheck = runSingboxCheck
	}
	out, err := runCheck(timeoutCtx, binaryPath, tmpPath)
	if err == nil {
		return Capability{V2RayAPIStats: true, Checked: true}
	}
	message := strings.TrimSpace(string(out))
	if message == "" {
		message = err.Error()
	}
	if IsV2RayAPIUnsupportedOutput(message) {
		return Capability{Checked: true, Unsupported: true, Message: StatsUnsupportedMessage}
	}
	return Capability{Checked: true, Message: message}
}

func IsV2RayAPIUnsupportedOutput(output string) bool {
	normalized := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(normalized, "v2ray api is not included") ||
		(strings.Contains(normalized, "with_v2ray_api") && strings.Contains(normalized, "not included"))
}

func runSingboxCheck(ctx context.Context, binaryPath, configPath string) ([]byte, error) {
	return exec.CommandContext(ctx, binaryPath, "check", "-c", configPath).CombinedOutput()
}

func localV2RayAPICheckListen() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 {
		return "", fmt.Errorf("unexpected listen address: %s", listener.Addr().String())
	}
	return fmt.Sprintf("127.0.0.1:%d", addr.Port), nil
}

func minimalV2RayAPICheckConfig(listen string) Config {
	return Config{
		Log: LogConfig{Level: "warn"},
		Outbounds: []OutboundConfig{
			{Type: "direct", Tag: "direct"},
		},
		Experimental: &ExperimentalConfig{V2RayAPI: &V2RayAPIConfig{
			Listen: listen,
			Stats:  &StatsAPIConfig{Enabled: true},
		}},
	}
}

func IsSingboxProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "hysteria2", "tuic", "shadowtls":
		return true
	default:
		return false
	}
}

func HasEnabledSingboxInbound(inbounds []db.Inbound) bool {
	for _, inbound := range inbounds {
		if inbound.Enabled && IsSingboxProtocol(inbound.Protocol) {
			return true
		}
	}
	return false
}

func InboundStatsTag(inbound db.Inbound) string {
	protocol := strings.ToLower(strings.TrimSpace(inbound.Protocol))
	switch protocol {
	case "hysteria2":
		return fmt.Sprintf("hy2-inbound-%d", inbound.ID)
	case "tuic":
		return fmt.Sprintf("tuic-inbound-%d", inbound.ID)
	case "shadowtls":
		return fmt.Sprintf("shadowtls-inbound-%d", inbound.ID)
	default:
		return fmt.Sprintf("inbound-%d-%s", inbound.ID, protocol)
	}
}

func UserStatsKey(client db.Client) string {
	key := strings.TrimSpace(client.StatsKey)
	if key != "" {
		return key
	}
	return strings.TrimSpace(client.Email)
}
