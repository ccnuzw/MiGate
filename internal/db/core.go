package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	CoreXray    = "xray"
	CoreSingbox = "sing-box"

	OutboundSupportBuiltin = "builtin"
	OutboundSupportFull    = "full"
	OutboundSupportBasic   = "basic"
	OutboundSupportNone    = "none"
)

var singboxInboundProtocols = map[string]bool{
	"hysteria2": true,
	"tuic":      true,
	"shadowtls": true,
}

var sharedOutboundProtocols = map[string]bool{
	"socks":       true,
	"socks5":      true,
	"http":        true,
	"https":       true,
	"vless":       true,
	"trojan":      true,
	"shadowsocks": true,
}

var singboxOnlyOutboundProtocols = map[string]bool{
	"hysteria2": true,
	"tuic":      true,
	"shadowtls": true,
}

var builtinOutboundProtocols = map[string]bool{
	"direct":    true,
	"block":     true,
	"dns":       true,
	"freedom":   true,
	"blackhole": true,
}

var fullOutboundProtocols = map[string]bool{
	"socks":  true,
	"socks5": true,
	"http":   true,
	"https":  true,
}

var basicOutboundProtocols = map[string]bool{
	"vless":       true,
	"trojan":      true,
	"shadowsocks": true,
	"hysteria2":   true,
	"tuic":        true,
	"shadowtls":   true,
}

func NormalizeCore(core string) string {
	switch strings.ToLower(strings.TrimSpace(core)) {
	case CoreSingbox, "singbox":
		return CoreSingbox
	default:
		return CoreXray
	}
}

func InferInboundCore(protocol string) string {
	if capability, ok := GetInboundCapability(protocol); ok {
		return capability.Core
	}
	return CoreXray
}

func InboundCore(inbound Inbound) string {
	if strings.TrimSpace(inbound.Core) == "" {
		return InferInboundCore(inbound.Protocol)
	}
	return NormalizeCore(inbound.Core)
}

func NormalizeOutboundProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "socks5":
		return "socks"
	case "direct":
		return "freedom"
	case "block":
		return "blackhole"
	default:
		return protocol
	}
}

func OutboundProtocolSupportedCores(protocol string) []string {
	normalized := NormalizeOutboundProtocol(protocol)
	switch {
	case sharedOutboundProtocols[normalized], builtinOutboundProtocols[normalized]:
		return []string{CoreXray, CoreSingbox}
	case singboxOnlyOutboundProtocols[normalized]:
		return []string{CoreSingbox}
	default:
		return []string{}
	}
}

func OutboundProtocolSupportLevel(protocol string) string {
	normalized := NormalizeOutboundProtocol(protocol)
	switch {
	case builtinOutboundProtocols[normalized]:
		return OutboundSupportBuiltin
	case fullOutboundProtocols[normalized]:
		return OutboundSupportFull
	case basicOutboundProtocols[normalized]:
		return OutboundSupportBasic
	default:
		return OutboundSupportNone
	}
}

func SupportsCore(cores []string, core string) bool {
	want := NormalizeCore(core)
	for _, item := range cores {
		if NormalizeCore(item) == want {
			return true
		}
	}
	return false
}

func OutboundSupportsCore(outbound Outbound, core string) bool {
	return SupportsCore(OutboundProtocolSupportedCores(outbound.Protocol), core)
}

func ResolveRuleOutbound(rule RoutingRule, outbounds []Outbound) (Outbound, bool) {
	if rule.OutboundID > 0 {
		for _, outbound := range outbounds {
			if outbound.ID == rule.OutboundID {
				return outbound, true
			}
		}
	}
	return Outbound{}, false
}

func EffectiveRuleOutboundID(rule RoutingRule, outbounds []Outbound) int64 {
	outbound, ok := ResolveRuleOutbound(rule, outbounds)
	if ok {
		return outbound.ID
	}
	return 0
}

func RuleTargetSupportsCore(rule RoutingRule, outbounds []Outbound, core string) bool {
	outbound, ok := ResolveRuleOutbound(rule, outbounds)
	return ok && OutboundSupportsCore(outbound, core)
}

func ValidateOutboundProfile(outbound Outbound) error {
	protocol := NormalizeOutboundProtocol(outbound.Protocol)
	if outboundProtocolNeedsEndpoint(protocol) {
		if strings.TrimSpace(outbound.Address) == "" {
			return fmt.Errorf("%s outbound requires address", protocol)
		}
		if outbound.Port <= 0 || outbound.Port > 65535 {
			return fmt.Errorf("%s outbound requires valid port", protocol)
		}
	}
	switch protocol {
	case "vless":
		if !IsUUID(outbound.Username) {
			return fmt.Errorf("vless outbound requires valid uuid in username")
		}
	case "trojan":
		if strings.TrimSpace(outbound.Password) == "" {
			return fmt.Errorf("trojan outbound requires password")
		}
	case "shadowsocks":
		if strings.TrimSpace(outbound.Username) == "" {
			return fmt.Errorf("shadowsocks outbound requires method in username")
		}
		if strings.TrimSpace(outbound.Password) == "" {
			return fmt.Errorf("shadowsocks outbound requires password")
		}
	case "hysteria2":
		if strings.TrimSpace(outbound.Password) == "" {
			return fmt.Errorf("hysteria2 outbound requires password")
		}
	case "tuic":
		if !IsUUID(outbound.Username) {
			return fmt.Errorf("tuic outbound requires valid uuid in username")
		}
		if strings.TrimSpace(outbound.Password) == "" {
			return fmt.Errorf("tuic outbound requires password")
		}
	case "shadowtls":
		if strings.TrimSpace(outbound.Password) == "" {
			return fmt.Errorf("shadowtls outbound requires password")
		}
	}
	return nil
}

func GeneratedOutboundTag(core string, profileID int64, fallback string) string {
	if profileID <= 0 {
		return strings.TrimSpace(fallback)
	}
	switch NormalizeCore(core) {
	case CoreSingbox:
		return fmt.Sprintf("singbox-out-%d", profileID)
	default:
		return fmt.Sprintf("xray-out-%d", profileID)
	}
}

type CoreApplyState struct {
	Core             string `json:"core"`
	LastAppliedHash  string `json:"last_applied_hash"`
	LastAppliedAt    string `json:"last_applied_at"`
	PendingDirty     bool   `json:"pending_dirty"`
	PendingReason    string `json:"pending_reason"`
	PendingUpdatedAt string `json:"pending_updated_at"`
}

func (s *Store) GetCoreApplyState(ctx context.Context, core string) (CoreApplyState, bool, error) {
	core = NormalizeCore(core)
	var state CoreApplyState
	var pendingDirty int
	err := s.db.QueryRowContext(ctx, `
SELECT core, last_applied_hash, last_applied_at, pending_dirty, pending_reason, pending_updated_at
FROM core_apply_state
WHERE core = ?
`, core).Scan(&state.Core, &state.LastAppliedHash, &state.LastAppliedAt, &pendingDirty, &state.PendingReason, &state.PendingUpdatedAt)
	if err == sql.ErrNoRows {
		return CoreApplyState{Core: core}, false, nil
	}
	if err != nil {
		return CoreApplyState{}, false, err
	}
	state.PendingDirty = pendingDirty != 0
	return state, true, nil
}

func (s *Store) MarkCoreApplied(ctx context.Context, core string, hash string, appliedAt time.Time) error {
	core = NormalizeCore(core)
	hash = strings.TrimSpace(hash)
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO core_apply_state (core, last_applied_hash, last_applied_at, pending_dirty, pending_reason, pending_updated_at)
VALUES (?, ?, ?, 0, '', '')
ON CONFLICT(core) DO UPDATE SET
  last_applied_hash = excluded.last_applied_hash,
  last_applied_at = excluded.last_applied_at,
  pending_dirty = 0,
  pending_reason = '',
  pending_updated_at = ''
`, core, hash, appliedAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) MarkCorePending(ctx context.Context, core string, reason string, updatedAt time.Time) error {
	core = NormalizeCore(core)
	reason = strings.TrimSpace(reason)
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO core_apply_state (core, last_applied_hash, last_applied_at, pending_dirty, pending_reason, pending_updated_at)
VALUES (?, '', '', 1, ?, ?)
ON CONFLICT(core) DO UPDATE SET
  pending_dirty = 1,
  pending_reason = excluded.pending_reason,
  pending_updated_at = excluded.pending_updated_at
`, core, reason, updatedAt.UTC().Format(time.RFC3339))
	return err
}

func OutboundProfileIDFromGeneratedTag(core string, tag string) (int64, bool) {
	normalized := NormalizeCore(core)
	prefix := "xray-out-"
	if normalized == CoreSingbox {
		prefix = "singbox-out-"
	}
	trimmed := strings.TrimSpace(tag)
	if !strings.HasPrefix(trimmed, prefix) {
		return 0, false
	}
	rawID := strings.TrimPrefix(trimmed, prefix)
	if rawID == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func RoutingRuleAppliesToCore(rule RoutingRule, inbounds []Inbound, core string) bool {
	want := NormalizeCore(core)
	if rule.ClientID > 0 {
		for _, inbound := range inbounds {
			for _, client := range inbound.Clients {
				if client.ID == rule.ClientID {
					return inbound.Enabled && InboundCore(inbound) == want
				}
			}
		}
		return false
	}
	if rule.InboundID > 0 {
		for _, inbound := range inbounds {
			if inbound.ID == rule.InboundID {
				return inbound.Enabled && InboundCore(inbound) == want
			}
		}
		return false
	}
	inboundTag := strings.TrimSpace(rule.InboundTag)
	if inboundTag == "" {
		for _, inbound := range inbounds {
			if inbound.Enabled && InboundCore(inbound) == want {
				return true
			}
		}
		return len(inbounds) == 0
	}
	for _, inbound := range inbounds {
		if GeneratedInboundTag(inbound) == inboundTag || strings.TrimSpace(inbound.Remark) == inboundTag {
			return inbound.Enabled && InboundCore(inbound) == want
		}
	}
	return false
}

func GeneratedInboundTag(inbound Inbound) string {
	return fmt.Sprintf("inbound-%d-%s", inbound.ID, strings.ToLower(strings.TrimSpace(inbound.Protocol)))
}

func IsUUID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}

func outboundProtocolNeedsEndpoint(protocol string) bool {
	switch protocol {
	case "socks", "http", "https", "vless", "trojan", "shadowsocks", "hysteria2", "tuic", "shadowtls":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
