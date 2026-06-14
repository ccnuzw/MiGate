package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/xray"
)

func isSingBoxProtocol(protocol string) bool {
	switch protocol {
	case "hysteria2", "tuic", "shadowtls":
		return true
	default:
		return false
	}
}

func summarizeTraffic(ctx context.Context, inbounds []db.Inbound, statsClient xray.StatsClient) (map[int64]inboundTrafficSummary, map[int64]clientTrafficSummary) {
	liveStats := map[string]*xray.ClientStats{}
	if statsClient != nil {
		if stats, err := statsClient.QueryAllStats(ctx); err == nil {
			liveStats = stats
		}
	}
	byInbound := map[int64]inboundTrafficSummary{}
	byClient := map[int64]clientTrafficSummary{}
	for _, inbound := range inbounds {
		inboundSummary := inboundTrafficSummary{Source: "db"}
		if isSingBoxProtocol(inbound.Protocol) {
			inboundSummary.Source = "unavailable"
		}
		for _, client := range inbound.Clients {
			clientSummary := clientTrafficSummary{Up: client.Up, Down: client.Down, Source: "db"}
			if isSingBoxProtocol(inbound.Protocol) {
				clientSummary.Source = "unavailable"
				clientSummary.Note = "sing-box realtime traffic stats are not yet wired"
			} else if stats, ok := liveStats[client.Email]; ok {
				clientSummary.XrayUp = stats.Uplink
				clientSummary.XrayDown = stats.Downlink
				clientSummary.RealtimeSource = "xray"
				inboundSummary.RealtimeSource = "xray"
			}
			byClient[client.ID] = clientSummary
			inboundSummary.Up += clientSummary.Up
			inboundSummary.Down += clientSummary.Down
		}
		inboundSummary.Total = inboundSummary.Up + inboundSummary.Down
		byInbound[inbound.ID] = inboundSummary
	}
	return byInbound, byClient
}

func statsHandler(store Store, statsClient xray.StatsClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}
		ctx := r.Context()
		inb, err := store.ListInbounds(ctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		obs, err := store.ListOutbounds(ctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_outbounds_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		rules, err := store.ListRoutingRules(ctx)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_routing_rules_failed", map[string]interface{}{"detail": err.Error()})
			return
		}
		var clientCount int
		for _, in := range inb {
			clientCount += len(in.Clients)
		}
		totalObs := len(obs)
		enabledObs := 0
		for _, ob := range obs {
			if ob.Enabled {
				enabledObs++
			}
		}
		totalRules := len(rules)
		enabledRules := 0
		for _, r := range rules {
			if r.Enabled {
				enabledRules++
			}
		}

		trafficByInbound, trafficByClient := summarizeTraffic(ctx, inb, statsClient)
		var clientList []map[string]interface{}
		var totalUp int64
		var totalDown int64
		for _, in := range inb {
			for _, c := range in.Clients {
				clientTraffic := trafficByClient[c.ID]
				info := map[string]interface{}{
					"id":                   c.ID,
					"inbound_id":           c.InboundID,
					"protocol":             in.Protocol,
					"email":                c.Email,
					"enabled":              c.Enabled,
					"up":                   clientTraffic.Up,
					"down":                 clientTraffic.Down,
					"xray_up":              clientTraffic.XrayUp,
					"xray_down":            clientTraffic.XrayDown,
					"traffic_limit":        c.TrafficLimit,
					"expiry_at":            c.ExpiryAt,
					"traffic_stats_source": clientTraffic.Source,
				}
				if clientTraffic.RealtimeSource != "" {
					info["realtime_stats_source"] = clientTraffic.RealtimeSource
				}
				if clientTraffic.Note != "" {
					info["traffic_stats_note"] = clientTraffic.Note
				}
				clientList = append(clientList, info)
			}
		}
		for _, traffic := range trafficByInbound {
			totalUp += traffic.Up
			totalDown += traffic.Down
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"inbounds":              len(inb),
			"clients":               clientCount,
			"client_details":        clientList,
			"traffic_up":            totalUp,
			"traffic_down":          totalDown,
			"traffic_total":         totalUp + totalDown,
			"outbounds":             totalObs,
			"outbounds_enabled":     enabledObs,
			"routing_rules":         totalRules,
			"routing_rules_enabled": enabledRules,
		})
	}
}

func dashboardSummaryHandler(store Store, statsClient xray.StatsClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
			return
		}
		summary, err := buildDashboardSummary(r.Context(), store, statsClient)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}

func buildDashboardSummary(ctx context.Context, store Store, statsClient xray.StatsClient) (map[string]interface{}, error) {
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_inbounds_failed")
	}
	outbounds, err := store.ListOutbounds(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_outbounds_failed")
	}
	rules, err := store.ListRoutingRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list_routing_rules_failed")
	}
	now := time.Now().Unix()
	clientCount := 0
	activeClients := 0
	expiredClients := 0
	limitedClients := 0
	enabledInbounds := 0
	protocols := map[string]int{}
	trafficSeries := make([]trafficSeriesPoint, 0, len(inbounds))
	trafficByInbound, trafficByClient := summarizeTraffic(ctx, inbounds, statsClient)
	var totalUp int64
	var totalDown int64
	var realtimeUp int64
	var realtimeDown int64
	for _, inbound := range inbounds {
		if inbound.Enabled {
			enabledInbounds++
		}
		if inbound.Protocol != "" {
			protocols[inbound.Protocol]++
		}
		if traffic, ok := trafficByInbound[inbound.ID]; ok {
			totalUp += traffic.Up
			totalDown += traffic.Down
			name := inbound.Remark
			if strings.TrimSpace(name) == "" {
				name = fmt.Sprintf("%s:%d", inbound.Protocol, inbound.Port)
			}
			trafficSeries = append(trafficSeries, trafficSeriesPoint{Name: name, Up: traffic.Up, Down: traffic.Down})
		}
		for _, client := range inbound.Clients {
			clientCount++
			used := client.Up + client.Down
			expired := client.ExpiryAt > 0 && client.ExpiryAt <= now
			limited := client.TrafficLimit > 0 && used >= client.TrafficLimit
			if expired {
				expiredClients++
			}
			if limited {
				limitedClients++
			}
			if client.Enabled && !expired && !limited {
				activeClients++
			}
			if traffic, ok := trafficByClient[client.ID]; ok {
				realtimeUp += traffic.XrayUp
				realtimeDown += traffic.XrayDown
			}
		}
	}
	enabledOutbounds := 0
	for _, outbound := range outbounds {
		if outbound.Enabled {
			enabledOutbounds++
		}
	}
	enabledRules := 0
	for _, rule := range rules {
		if rule.Enabled {
			enabledRules++
		}
	}
	return map[string]interface{}{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"counts": map[string]int{
			"inbounds":          len(inbounds),
			"inbounds_enabled":  enabledInbounds,
			"clients":           clientCount,
			"clients_active":    activeClients,
			"clients_expired":   expiredClients,
			"clients_limited":   limitedClients,
			"outbounds":         len(outbounds),
			"outbounds_enabled": enabledOutbounds,
			"routing_rules":     len(rules),
			"routing_enabled":   enabledRules,
		},
		"traffic": map[string]int64{
			"up":            totalUp,
			"down":          totalDown,
			"total":         totalUp + totalDown,
			"xray_up":       realtimeUp,
			"xray_down":     realtimeDown,
			"xray_realtime": realtimeUp + realtimeDown,
		},
		"protocols":      protocols,
		"traffic_series": trafficSeries,
		"validation": map[string]configValidationResult{
			"xray":    validateXrayConfig(ctx, store),
			"singbox": validateSingboxConfig(ctx, store),
		},
	}, nil
}

type cpuSample struct {
	Idle  uint64
	Total uint64
}

func systemResourcesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		memTotal, memUsed, memPercent := readMemoryUsage()
		diskTotal, diskUsed, diskPercent := readDiskUsage("/")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"cpu_percent":    sampleCPUPercent(),
			"memory_total":   memTotal,
			"memory_used":    memUsed,
			"memory_percent": memPercent,
			"disk_total":     diskTotal,
			"disk_used":      diskUsed,
			"disk_percent":   diskPercent,
			"uptime_seconds": readUptimeSeconds(),
		})
	}
}

func sampleCPUPercent() float64 {
	first, err := readCPUSample()
	if err != nil {
		return 0
	}
	time.Sleep(100 * time.Millisecond)
	second, err := readCPUSample()
	if err != nil || second.Total <= first.Total {
		return 0
	}
	totalDelta := second.Total - first.Total
	idleDelta := second.Idle - first.Idle
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0
	}
	return clampPercent(round1(float64(totalDelta-idleDelta) * 100 / float64(totalDelta)))
}

func readCPUSample() (cpuSample, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuSample{}, fmt.Errorf("invalid cpu stat")
	}
	var total uint64
	var idle uint64
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return cpuSample{}, err
		}
		total += value
		if i == 3 || i == 4 {
			idle += value
		}
	}
	return cpuSample{Idle: idle, Total: total}, nil
}

func readMemoryUsage() (totalBytes, usedBytes int64, percent float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, 0
	}
	values := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total <= 0 || available < 0 {
		return 0, 0, 0
	}
	used := total - available
	return total, used, clampPercent(round1(float64(used) * 100 / float64(total)))
}

func readDiskUsage(path string) (totalBytes, usedBytes int64, percent float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	free := int64(stat.Bavail) * int64(stat.Bsize)
	if total <= 0 || free < 0 {
		return 0, 0, 0
	}
	used := total - free
	return total, used, clampPercent(round1(float64(used) * 100 / float64(total)))
}

func readUptimeSeconds() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
