package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCapabilityDetectUnsupportedV2RayAPI(t *testing.T) {
	detector := NewCapabilityDetector("/tmp/sing-box")
	var checkedPath string
	detector.runCheck = func(ctx context.Context, binaryPath, configPath string) ([]byte, error) {
		if !strings.HasPrefix(configPath, "/") {
			t.Fatalf("expected temp config path, got %q", configPath)
		}
		assertCapabilityCheckConfigUsesDynamicListen(t, configPath)
		checkedPath = configPath
		return []byte("FATAL create v2ray-server: v2ray api is not included in this build, rebuild with -tags with_v2ray_api"), errors.New("exit status 1")
	}

	got := detector.Detect(context.Background())
	if got.V2RayAPIStats || !got.Unsupported || got.Message != StatsUnsupportedMessage {
		t.Fatalf("expected unsupported capability, got %+v", got)
	}
	assertTempConfigRemoved(t, checkedPath)
}

func TestCapabilityDetectSupportedV2RayAPI(t *testing.T) {
	detector := NewCapabilityDetector("/tmp/sing-box")
	var checkedPath string
	detector.runCheck = func(ctx context.Context, binaryPath, configPath string) ([]byte, error) {
		assertCapabilityCheckConfigUsesDynamicListen(t, configPath)
		checkedPath = configPath
		return []byte("ok"), nil
	}

	got := detector.Detect(context.Background())
	if !got.V2RayAPIStats || !got.Checked || got.Unsupported {
		t.Fatalf("expected supported capability, got %+v", got)
	}
	assertTempConfigRemoved(t, checkedPath)
}

func TestCapabilityDetectUnsupportedWithV2RayAPINotIncludedOutput(t *testing.T) {
	detector := NewCapabilityDetector("/tmp/sing-box")
	detector.runCheck = func(ctx context.Context, binaryPath, configPath string) ([]byte, error) {
		return []byte("with_v2ray_api not included"), errors.New("exit status 1")
	}

	got := detector.Detect(context.Background())
	if got.V2RayAPIStats || !got.Checked || !got.Unsupported || got.Message != StatsUnsupportedMessage {
		t.Fatalf("expected unsupported capability, got %+v", got)
	}
}

func TestCapabilityDetectOtherErrorKeepsMessage(t *testing.T) {
	detector := NewCapabilityDetector("/tmp/sing-box")
	detector.runCheck = func(ctx context.Context, binaryPath, configPath string) ([]byte, error) {
		return []byte("invalid config: something else failed"), errors.New("exit status 1")
	}

	got := detector.Detect(context.Background())
	if got.V2RayAPIStats || !got.Checked || got.Unsupported || got.Message != "invalid config: something else failed" {
		t.Fatalf("expected checked error message, got %+v", got)
	}
}

func assertCapabilityCheckConfigUsesDynamicListen(t *testing.T, configPath string) {
	t.Helper()
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read temp config: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode temp config: %v", err)
	}
	experimental, ok := decoded["experimental"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected experimental config: %s", raw)
	}
	api, ok := experimental["v2ray_api"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected v2ray_api config: %s", raw)
	}
	listen, ok := api["listen"].(string)
	if !ok || listen == "" {
		t.Fatalf("expected v2ray_api listen string, got %#v", api["listen"])
	}
	if listen == "127.0.0.1:10086" {
		t.Fatalf("capability check must not use fixed listen address: %s", listen)
	}
	if !strings.HasPrefix(listen, "127.0.0.1:") {
		t.Fatalf("expected loopback listen address, got %q", listen)
	}
}

func assertTempConfigRemoved(t *testing.T, configPath string) {
	t.Helper()
	if configPath == "" {
		t.Fatalf("expected captured temp config path")
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp config to be removed, stat err=%v", err)
	}
}
