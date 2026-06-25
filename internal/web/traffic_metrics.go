package web

type TrafficCumulativeMetric struct {
	Up      int64  `json:"up"`
	Down    int64  `json:"down"`
	Total   int64  `json:"total"`
	Status  string `json:"status"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

type TrafficRealtimeMetric struct {
	DeltaUp       int64   `json:"delta_up"`
	DeltaDown     int64   `json:"delta_down"`
	DeltaTotal    int64   `json:"delta_total"`
	RateUp        float64 `json:"rate_up"`
	RateDown      float64 `json:"rate_down"`
	RateTotal     float64 `json:"rate_total"`
	ObservedAt    string  `json:"observed_at"`
	WindowSeconds float64 `json:"window_seconds"`
	Status        string  `json:"status"`
	Source        string  `json:"source"`
	Message       string  `json:"message"`
}

type TrafficMetricSet struct {
	TotalCumulative   TrafficCumulativeMetric           `json:"total_cumulative"`
	TotalRealtime     TrafficRealtimeMetric             `json:"total_realtime_traffic"`
	InboundCumulative map[int64]TrafficCumulativeMetric `json:"inbound_cumulative"`
	InboundRealtime   map[int64]TrafficRealtimeMetric   `json:"inbound_realtime"`
	ClientCumulative  map[int64]TrafficCumulativeMetric `json:"client_cumulative"`
	ClientRealtime    map[int64]TrafficRealtimeMetric   `json:"client_realtime"`
}

func newTrafficCumulativeMetric(up, down int64, status, source, message string) TrafficCumulativeMetric {
	return TrafficCumulativeMetric{
		Up:      up,
		Down:    down,
		Total:   up + down,
		Status:  normalizedMetricStatus(status),
		Source:  normalizedMetricSource(source),
		Message: message,
	}
}

func newTrafficRealtimeMetric(deltaUp, deltaDown int64, rateUp, rateDown, windowSeconds float64, observedAt, status, source, message string) TrafficRealtimeMetric {
	return TrafficRealtimeMetric{
		DeltaUp:       deltaUp,
		DeltaDown:     deltaDown,
		DeltaTotal:    deltaUp + deltaDown,
		RateUp:        rateUp,
		RateDown:      rateDown,
		RateTotal:     rateUp + rateDown,
		ObservedAt:    observedAt,
		WindowSeconds: windowSeconds,
		Status:        normalizedMetricStatus(status),
		Source:        normalizedMetricSource(source),
		Message:       message,
	}
}

func cumulativeMetricPayload(metric TrafficCumulativeMetric) map[string]interface{} {
	return map[string]interface{}{
		"up":      metric.Up,
		"down":    metric.Down,
		"total":   metric.Total,
		"status":  metric.Status,
		"source":  metric.Source,
		"message": metric.Message,
	}
}

func realtimeMetricPayload(metric TrafficRealtimeMetric) map[string]interface{} {
	return map[string]interface{}{
		"delta_up":       metric.DeltaUp,
		"delta_down":     metric.DeltaDown,
		"delta_total":    metric.DeltaTotal,
		"rate_up":        metric.RateUp,
		"rate_down":      metric.RateDown,
		"rate_total":     metric.RateTotal,
		"observed_at":    metric.ObservedAt,
		"window_seconds": metric.WindowSeconds,
		"status":         metric.Status,
		"source":         metric.Source,
		"message":        metric.Message,
	}
}

func buildTrafficMetricSet(view trafficView) TrafficMetricSet {
	metrics := TrafficMetricSet{
		InboundCumulative: map[int64]TrafficCumulativeMetric{},
		InboundRealtime:   map[int64]TrafficRealtimeMetric{},
		ClientCumulative:  map[int64]TrafficCumulativeMetric{},
		ClientRealtime:    map[int64]TrafficRealtimeMetric{},
	}
	var totalUp int64
	var totalDown int64
	var totalDeltaUp int64
	var totalDeltaDown int64
	var totalRateUp float64
	var totalRateDown float64
	var totalWindowSeconds float64
	realtimeCounts := trafficCoverageCounts{}
	for _, inbound := range view.inbounds {
		traffic := view.trafficByInbound[inbound.ID]
		cumulative := newTrafficCumulativeMetric(traffic.Up, traffic.Down, traffic.Status, traffic.Source, traffic.Message)
		realtime := newInboundRealtimeMetric(traffic)
		metrics.InboundCumulative[inbound.ID] = cumulative
		metrics.InboundRealtime[inbound.ID] = realtime
		realtimeCounts.add(realtime.Status)
		totalUp += cumulative.Up
		totalDown += cumulative.Down
		totalDeltaUp += realtime.DeltaUp
		totalDeltaDown += realtime.DeltaDown
		totalRateUp += realtime.RateUp
		totalRateDown += realtime.RateDown
		if realtime.WindowSeconds > totalWindowSeconds {
			totalWindowSeconds = realtime.WindowSeconds
		}
		for _, client := range inbound.Clients {
			clientTraffic := view.trafficByClient[client.ID]
			metrics.ClientCumulative[client.ID] = newTrafficCumulativeMetric(clientTraffic.Up, clientTraffic.Down, clientTraffic.Status, clientTraffic.Source, clientTraffic.Message)
			metrics.ClientRealtime[client.ID] = newTrafficRealtimeMetric(clientTraffic.DeltaUp, clientTraffic.DeltaDown, clientTraffic.RateUp, clientTraffic.RateDown, clientTraffic.WindowSeconds, clientTraffic.LastSampledAt, clientTraffic.Status, clientTraffic.Source, clientTraffic.Message)
		}
	}
	coverage := buildTrafficCoverage(view.trafficByInbound)
	overallStatus, _ := coverage["overall"].(string)
	totalCumulativeSource, totalCumulativeMessage := totalCumulativeMetadata(metrics.InboundCumulative)
	metrics.TotalCumulative = newTrafficCumulativeMetric(totalUp, totalDown, overallStatus, totalCumulativeSource, totalCumulativeMessage)
	totalRealtimeSource, totalRealtimeMessage := totalRealtimeMetadata(metrics.InboundRealtime)
	metrics.TotalRealtime = newTrafficRealtimeMetric(totalDeltaUp, totalDeltaDown, totalRateUp, totalRateDown, totalWindowSeconds, lastTrafficSampledAt(view.trafficByInbound, nil), realtimeCounts.status(), totalRealtimeSource, totalRealtimeMessage)
	return metrics
}

func totalCumulativeMetadata(inbounds map[int64]TrafficCumulativeMetric) (string, string) {
	if len(inbounds) == 0 {
		return "migate", ""
	}
	sources := map[string]struct{}{}
	for _, metric := range inbounds {
		source := normalizedMetricSource(metric.Source)
		sources[source] = struct{}{}
	}
	if len(sources) == 1 {
		for source := range sources {
			return source, ""
		}
	}
	return "mixed", ""
}

func totalRealtimeMetadata(inbounds map[int64]TrafficRealtimeMetric) (string, string) {
	if len(inbounds) == 0 {
		return "migate", ""
	}
	sources := map[string]struct{}{}
	for _, metric := range inbounds {
		source := normalizedMetricSource(metric.Source)
		sources[source] = struct{}{}
	}
	if len(sources) == 1 {
		for source := range sources {
			return source, ""
		}
	}
	return "mixed", ""
}

func newInboundRealtimeMetric(traffic inboundTrafficSummary) TrafficRealtimeMetric {
	return newTrafficRealtimeMetric(traffic.DeltaUp, traffic.DeltaDown, traffic.RateUp, traffic.RateDown, traffic.WindowSeconds, traffic.LastSampledAt, traffic.Status, traffic.Source, traffic.Message)
}

func normalizedMetricStatus(status string) string {
	if status == "" {
		return "waiting"
	}
	return status
}

func normalizedMetricSource(source string) string {
	if source == "" {
		return "migate"
	}
	return source
}
