package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *Store) ResetClientTraffic(ctx context.Context, id int64) (Client, error) {
	return s.ResetClientTrafficBaseline(ctx, id, nil)
}

// UpdateClientTraffic only accepts a client stats_key as the identity input.
func (s *Store) UpdateClientTraffic(ctx context.Context, statsKey string, uplink, downlink int64) error {
	statsKey = strings.TrimSpace(statsKey)
	if statsKey == "" {
		return fmt.Errorf("client stats_key is required")
	}
	matchedStatsKey, engine, err := lookupClientTrafficIdentity(ctx, s.db, statsKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("client stats_key not found: %s", statsKey)
		}
		return err
	}
	if strings.TrimSpace(matchedStatsKey) == "" {
		return fmt.Errorf("client stats_key not found: %s", statsKey)
	}
	return s.ApplyTrafficRawStats(ctx, []TrafficRawStat{{Engine: engine, ScopeType: "client", ScopeKey: matchedStatsKey, RawUp: uplink, RawDown: downlink, Status: "ok"}}, time.Now().UTC())
}

func (s *Store) UpdateClientTrafficBatch(ctx context.Context, stats map[string]ClientTrafficUpdate) error {
	if len(stats) == 0 {
		return nil
	}
	keys := make([]string, 0, len(stats))
	for statsKey := range stats {
		statsKey = strings.TrimSpace(statsKey)
		if statsKey == "" {
			return fmt.Errorf("client stats_key is required")
		}
		keys = append(keys, statsKey)
	}
	knownStatsKeys, err := lookupClientStatsKeys(ctx, s.db, keys)
	if err != nil {
		return err
	}
	raw := make([]TrafficRawStat, 0, len(stats))
	for statsKey, traffic := range stats {
		statsKey = strings.TrimSpace(statsKey)
		matchedStatsKey := knownStatsKeys[statsKey]
		if strings.TrimSpace(matchedStatsKey) == "" {
			return fmt.Errorf("client stats_key not found: %s", statsKey)
		}
		engine, _, err := lookupClientExpectedTrafficEngine(ctx, s.db, matchedStatsKey)
		if err != nil {
			return err
		}
		if strings.TrimSpace(engine) == "" {
			engine = "xray"
		}
		raw = append(raw, TrafficRawStat{Engine: engine, ScopeType: "client", ScopeKey: matchedStatsKey, RawUp: traffic.Up, RawDown: traffic.Down, Status: "ok"})
	}
	return s.ApplyTrafficRawStats(ctx, raw, time.Now().UTC())
}

func (s *Store) ApplyTrafficRawStats(ctx context.Context, stats []TrafficRawStat, observedAt time.Time) error {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	normalizedStats := normalizeTrafficRawStats(stats)
	if len(normalizedStats) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	currentStates, err := prefetchTrafficStates(ctx, tx, normalizedStats)
	if err != nil {
		return err
	}
	seenAt := observedAt.UTC().Format(time.RFC3339Nano)
	sampleBucketAt := trafficSampleBucket(observedAt).UTC().Format(time.RFC3339Nano)
	upsertState, err := tx.PrepareContext(ctx, `
INSERT INTO traffic_states (engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, delta_up, delta_down, rate_up, rate_down, window_seconds, last_seen_at, status, message)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(engine, scope_type, scope_key) DO UPDATE SET
  total_up=excluded.total_up,
  total_down=excluded.total_down,
  last_raw_up=excluded.last_raw_up,
  last_raw_down=excluded.last_raw_down,
  delta_up=excluded.delta_up,
  delta_down=excluded.delta_down,
  rate_up=excluded.rate_up,
  rate_down=excluded.rate_down,
  window_seconds=excluded.window_seconds,
  last_seen_at=excluded.last_seen_at,
  status=excluded.status,
  message=excluded.message
`)
	if err != nil {
		return err
	}
	defer upsertState.Close()
	insertSample, err := tx.PrepareContext(ctx, `
INSERT INTO traffic_samples (sampled_at, engine, scope_type, scope_key, total_up, total_down, delta_up, delta_down, rate_up, rate_down, window_seconds, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(sampled_at, engine, scope_type, scope_key) DO UPDATE SET
  total_up=excluded.total_up,
  total_down=excluded.total_down,
  delta_up=excluded.delta_up,
  delta_down=excluded.delta_down,
  rate_up=excluded.rate_up,
  rate_down=excluded.rate_down,
  window_seconds=excluded.window_seconds,
  status=excluded.status
`)
	if err != nil {
		return err
	}
	defer insertSample.Close()
	for _, raw := range normalizedStats {
		stateKey := trafficStateKey(raw.Engine, raw.ScopeType, raw.ScopeKey)
		current, hasCurrent := currentStates[stateKey]
		var elapsed float64
		if hasCurrent && current.LastSeenAt != "" {
			if previous, parseErr := time.Parse(time.RFC3339Nano, current.LastSeenAt); parseErr == nil && observedAt.After(previous) {
				elapsed = observedAt.Sub(previous).Seconds()
			}
		}
		deltaUp := int64(0)
		deltaDown := int64(0)
		if !hasCurrent || isMissingRawBaseline(current) {
			deltaUp = 0
			deltaDown = 0
		} else {
			if raw.RawUp >= current.LastRawUp {
				deltaUp = raw.RawUp - current.LastRawUp
			}
			if raw.RawDown >= current.LastRawDown {
				deltaDown = raw.RawDown - current.LastRawDown
			}
		}
		totalUp := current.TotalUp + deltaUp
		totalDown := current.TotalDown + deltaDown
		rateUp := 0.0
		rateDown := 0.0
		if elapsed > 0 && !shouldSuppressRecoveredTrafficRate(current) {
			rateUp = float64(deltaUp) / elapsed
			rateDown = float64(deltaDown) / elapsed
		}
		windowSeconds := elapsed
		message := strings.TrimSpace(raw.Message)
		if deltaUp == 0 && deltaDown == 0 && hasCurrent && isCounterReset(raw.RawUp, raw.RawDown, current.LastRawUp, current.LastRawDown) {
			if message == "" {
				message = "counter reset detected, baseline re-established"
			}
		}
		if _, err := upsertState.ExecContext(ctx, raw.Engine, raw.ScopeType, raw.ScopeKey, totalUp, totalDown, raw.RawUp, raw.RawDown, deltaUp, deltaDown, rateUp, rateDown, windowSeconds, seenAt, raw.Status, message); err != nil {
			return err
		}
		if _, err := insertSample.ExecContext(ctx, sampleBucketAt, raw.Engine, raw.ScopeType, raw.ScopeKey, totalUp, totalDown, deltaUp, deltaDown, rateUp, rateDown, windowSeconds, raw.Status); err != nil {
			return err
		}
		current.Engine = raw.Engine
		current.ScopeType = raw.ScopeType
		current.ScopeKey = raw.ScopeKey
		current.TotalUp = totalUp
		current.TotalDown = totalDown
		current.LastRawUp = raw.RawUp
		current.LastRawDown = raw.RawDown
		current.DeltaUp = deltaUp
		current.DeltaDown = deltaDown
		current.RateUp = rateUp
		current.RateDown = rateDown
		current.WindowSeconds = windowSeconds
		current.LastSeenAt = seenAt
		current.Status = raw.Status
		current.Message = strings.TrimSpace(raw.Message)
		currentStates[stateKey] = current
	}
	if err := repairPollutedSingleClientTraffic(ctx, tx, currentStates, normalizedStats, sampleBucketAt, seenAt); err != nil {
		return err
	}
	rollbackCleanup, err := s.cleanupTrafficSamples(ctx, tx, observedAt)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		rollbackCleanup()
		return err
	}
	return nil
}

func repairPollutedSingleClientTraffic(ctx context.Context, tx *sql.Tx, currentStates map[string]TrafficState, stats []TrafficRawStat, sampleBucketAt, seenAt string) error {
	observedInboundKeys := map[string]struct{}{}
	for _, raw := range stats {
		if raw.Engine == "xray" && raw.ScopeType == "inbound" {
			observedInboundKeys[raw.ScopeKey] = struct{}{}
		}
	}
	if len(observedInboundKeys) == 0 {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT i.id, i.protocol, c.stats_key
FROM inbounds i
JOIN clients c ON c.inbound_id = i.id
WHERE i.enabled = 1 AND c.enabled = 1 AND i.protocol NOT IN ('hysteria2', 'tuic', 'shadowtls', 'wireguard')
  AND (SELECT COUNT(*) FROM clients c2 WHERE c2.inbound_id = i.id AND c2.enabled = 1) = 1
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type singleClientInbound struct {
		inboundKey string
		clientKey  string
	}
	candidates := []singleClientInbound{}
	for rows.Next() {
		var inboundID int64
		var protocol, clientKey string
		if err := rows.Scan(&inboundID, &protocol, &clientKey); err != nil {
			return err
		}
		inboundKey := fmt.Sprintf("inbound-%d-%s", inboundID, strings.ToLower(strings.TrimSpace(protocol)))
		if _, ok := observedInboundKeys[inboundKey]; ok && strings.TrimSpace(clientKey) != "" {
			candidates = append(candidates, singleClientInbound{inboundKey: inboundKey, clientKey: strings.TrimSpace(clientKey)})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, candidate := range candidates {
		inboundState, hasInbound := currentStates[trafficStateKey("xray", "inbound", candidate.inboundKey)]
		clientKey := trafficStateKey("xray", "client", candidate.clientKey)
		clientState, hasClient := currentStates[clientKey]
		if !hasInbound || !hasClient || inboundState.Status != "ok" || clientState.Status != "ok" {
			continue
		}
		if clientState.TotalUp <= inboundState.TotalUp && clientState.TotalDown <= inboundState.TotalDown {
			continue
		}
		message := "client totals reconciled to native inbound counters because stored client totals exceeded the inbound total"
		_, err := tx.ExecContext(ctx, `
UPDATE traffic_states
SET total_up=?, total_down=?, delta_up=?, delta_down=?, rate_up=?, rate_down=?, window_seconds=?, last_seen_at=?, status='partial', message=?
WHERE engine='xray' AND scope_type='client' AND scope_key=?
`, inboundState.TotalUp, inboundState.TotalDown, 0, 0, 0, 0, 0, seenAt, message, candidate.clientKey)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO traffic_samples (sampled_at, engine, scope_type, scope_key, total_up, total_down, delta_up, delta_down, rate_up, rate_down, window_seconds, status)
VALUES (?, 'xray', 'client', ?, ?, ?, ?, ?, ?, ?, ?, 'partial')
ON CONFLICT(sampled_at, engine, scope_type, scope_key) DO UPDATE SET
  total_up=excluded.total_up,
  total_down=excluded.total_down,
  delta_up=excluded.delta_up,
  delta_down=excluded.delta_down,
  rate_up=excluded.rate_up,
  rate_down=excluded.rate_down,
  window_seconds=excluded.window_seconds,
  status=excluded.status
`, sampleBucketAt, candidate.clientKey, inboundState.TotalUp, inboundState.TotalDown, 0, 0, 0, 0, 0)
		if err != nil {
			return err
		}
		clientState.TotalUp = inboundState.TotalUp
		clientState.TotalDown = inboundState.TotalDown
		clientState.DeltaUp = 0
		clientState.DeltaDown = 0
		clientState.RateUp = 0
		clientState.RateDown = 0
		clientState.WindowSeconds = 0
		clientState.LastSeenAt = seenAt
		clientState.Status = "partial"
		clientState.Message = message
		currentStates[clientKey] = clientState
	}
	return nil
}

func normalizeTrafficRawStats(stats []TrafficRawStat) []TrafficRawStat {
	normalized := make([]TrafficRawStat, 0, len(stats))
	for _, raw := range stats {
		raw.Engine = normalizeTrafficEngine(raw.Engine)
		raw.ScopeType = normalizeTrafficToken(raw.ScopeType)
		raw.ScopeKey = strings.TrimSpace(raw.ScopeKey)
		raw.Status = strings.TrimSpace(raw.Status)
		raw.Message = strings.TrimSpace(raw.Message)
		if raw.Status == "" {
			raw.Status = "ok"
		}
		if raw.Engine == "" || raw.ScopeType == "" || raw.ScopeKey == "" {
			continue
		}
		normalized = append(normalized, raw)
	}
	return normalized
}

func trafficStateKey(engine, scopeType, scopeKey string) string {
	return engine + "\x00" + scopeType + "\x00" + scopeKey
}

func hasClientTrafficRawStats(stats []TrafficRawStat) bool {
	for _, raw := range stats {
		if raw.ScopeType == "client" {
			return true
		}
	}
	return false
}

func lookupClientStatsKeys(ctx context.Context, querier interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}, statsKeys []string) (map[string]string, error) {
	known := make(map[string]string, len(statsKeys))
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(statsKeys))
	for _, statsKey := range statsKeys {
		statsKey = strings.TrimSpace(statsKey)
		if statsKey == "" {
			continue
		}
		if _, ok := seen[statsKey]; ok {
			continue
		}
		seen[statsKey] = struct{}{}
		keys = append(keys, statsKey)
	}
	for start := 0; start < len(keys); start += sqliteVariableChunkSize {
		end := start + sqliteVariableChunkSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[start:end]
		args := make([]interface{}, 0, len(chunk))
		for _, statsKey := range chunk {
			args = append(args, statsKey)
		}
		rows, err := querier.QueryContext(ctx, `SELECT stats_key FROM clients WHERE stats_key IN (`+placeholders(len(chunk))+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var statsKey string
			if err := rows.Scan(&statsKey); err != nil {
				rows.Close()
				return nil, err
			}
			statsKey = strings.TrimSpace(statsKey)
			if statsKey != "" {
				known[statsKey] = statsKey
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return known, nil
}

func prefetchTrafficStates(ctx context.Context, tx *sql.Tx, stats []TrafficRawStat) (map[string]TrafficState, error) {
	keys := make([]string, 0, len(stats))
	seen := map[string]struct{}{}
	for _, raw := range stats {
		key := trafficStateKey(raw.Engine, raw.ScopeType, raw.ScopeKey)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	states := map[string]TrafficState{}
	for start := 0; start < len(keys); start += sqliteVariableChunkSize / 3 {
		end := start + sqliteVariableChunkSize/3
		if end > len(keys) {
			end = len(keys)
		}
		conditions := make([]string, 0, end-start)
		args := make([]interface{}, 0, (end-start)*3)
		for _, key := range keys[start:end] {
			parts := strings.SplitN(key, "\x00", 3)
			conditions = append(conditions, "(engine=? AND scope_type=? AND scope_key=?)")
			args = append(args, parts[0], parts[1], parts[2])
		}
		rows, err := tx.QueryContext(ctx, `
SELECT engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, delta_up, delta_down, rate_up, rate_down, window_seconds, last_seen_at, status, message
FROM traffic_states
WHERE `+strings.Join(conditions, " OR "), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var state TrafficState
			if err := rows.Scan(&state.Engine, &state.ScopeType, &state.ScopeKey, &state.TotalUp, &state.TotalDown, &state.LastRawUp, &state.LastRawDown, &state.DeltaUp, &state.DeltaDown, &state.RateUp, &state.RateDown, &state.WindowSeconds, &state.LastSeenAt, &state.Status, &state.Message); err != nil {
				rows.Close()
				return nil, err
			}
			state.Engine = normalizeTrafficEngine(state.Engine)
			state.ScopeType = normalizeTrafficToken(state.ScopeType)
			state.ScopeKey = strings.TrimSpace(state.ScopeKey)
			states[trafficStateKey(state.Engine, state.ScopeType, state.ScopeKey)] = state
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return states, nil
}

const (
	trafficSampleBucketSize       = 5 * time.Second
	trafficSamplesHotRetention    = 24 * time.Hour
	trafficSamplesRetention       = 30 * 24 * time.Hour
	trafficSamplesCleanupInterval = time.Hour
)

func trafficSampleBucket(observedAt time.Time) time.Time {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return observedAt.UTC().Truncate(trafficSampleBucketSize)
}

func (s *Store) cleanupTrafficSamples(ctx context.Context, tx *sql.Tx, observedAt time.Time) (func(), error) {
	s.trafficCleanupMu.Lock()
	if !s.nextTrafficSamplesCleanup.IsZero() && observedAt.Before(s.nextTrafficSamplesCleanup) {
		s.trafficCleanupMu.Unlock()
		return func() {}, nil
	}
	previousCleanup := s.nextTrafficSamplesCleanup
	nextCleanup := observedAt.Add(trafficSamplesCleanupInterval)
	s.nextTrafficSamplesCleanup = nextCleanup
	s.trafficCleanupMu.Unlock()
	cutoff := observedAt.Add(-trafficSamplesRetention).UTC().Format(time.RFC3339Nano)
	hotCutoff := observedAt.Add(-trafficSamplesHotRetention).UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
DELETE FROM traffic_samples
WHERE sampled_at < ?
   OR (sampled_at < ? AND (
     CAST(strftime('%M', sampled_at) AS INTEGER) % 5 <> 0
     OR CAST(strftime('%S', sampled_at) AS INTEGER) <> 0
   ))
`, cutoff, hotCutoff); err != nil {
		s.trafficCleanupMu.Lock()
		if s.nextTrafficSamplesCleanup.Equal(nextCleanup) {
			s.nextTrafficSamplesCleanup = previousCleanup
		}
		s.trafficCleanupMu.Unlock()
		return func() {}, err
	}
	return func() {
		s.trafficCleanupMu.Lock()
		if s.nextTrafficSamplesCleanup.Equal(nextCleanup) {
			s.nextTrafficSamplesCleanup = previousCleanup
		}
		s.trafficCleanupMu.Unlock()
	}, nil
}

func isMissingRawBaseline(state TrafficState) bool {
	return state.LastRawUp == 0 && state.LastRawDown == 0 &&
		state.DeltaUp == 0 && state.DeltaDown == 0 &&
		state.RateUp == 0 && state.RateDown == 0 &&
		state.WindowSeconds == 0 &&
		isBaselineOnlyTrafficStatus(state)
}

func isBaselineOnlyTrafficStatus(state TrafficState) bool {
	status := normalizeTrafficStatus(state.Status)
	if status == "waiting" || status == "not_configured" || status == "unsupported" {
		return true
	}
	if status == "ok" {
		return true
	}
	if status == "unavailable" {
		message := strings.ToLower(strings.TrimSpace(state.Message))
		return message == "" || strings.Contains(message, "baseline unavailable") || strings.Contains(message, "stats offline") || strings.Contains(message, "waiting")
	}
	return false
}

// isCounterReset detects when a core counter has been reset (e.g. Xray/sing-box restart).
// A reset is identified by new raw counters being lower than the last known values
// while the previous state had non-zero raw counters.
func isCounterReset(rawUp, rawDown, lastRawUp, lastRawDown int64) bool {
	if lastRawUp > 0 && rawUp < lastRawUp {
		return true
	}
	if lastRawDown > 0 && rawDown < lastRawDown {
		return true
	}
	return false
}

func shouldSuppressRecoveredTrafficRate(state TrafficState) bool {
	switch normalizeTrafficStatus(state.Status) {
	case "waiting", "unavailable", "unsupported", "not_configured":
		return true
	default:
		return false
	}
}

func (s *Store) MarkTrafficUnavailable(ctx context.Context, engine, status, message string, observedAt time.Time) error {
	engine = normalizeTrafficToken(engine)
	if engine == "" {
		return nil
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = "unavailable"
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE traffic_states SET delta_up=0, delta_down=0, rate_up=0, rate_down=0, window_seconds=0, status=?, message=?, last_seen_at=? WHERE engine=?`,
		status, strings.TrimSpace(message), observedAt.UTC().Format(time.RFC3339Nano), engine)
	return err
}

func (s *Store) MarkTrafficScopeStatus(ctx context.Context, stats []TrafficStatusMarker, observedAt time.Time) error {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	normalizedStats := normalizeTrafficStatusMarkers(stats)
	if len(normalizedStats) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	currentStates, err := prefetchTrafficStates(ctx, tx, trafficRawStatsForStatusMarkers(normalizedStats))
	if err != nil {
		return err
	}
	seenAt := observedAt.UTC().Format(time.RFC3339Nano)
	sampleBucketAt := trafficSampleBucket(observedAt).UTC().Format(time.RFC3339Nano)
	insertSample, err := tx.PrepareContext(ctx, `
INSERT INTO traffic_samples (sampled_at, engine, scope_type, scope_key, total_up, total_down, delta_up, delta_down, rate_up, rate_down, window_seconds, status)
VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, 0, 0, ?)
ON CONFLICT(sampled_at, engine, scope_type, scope_key) DO UPDATE SET
  total_up=excluded.total_up,
  total_down=excluded.total_down,
  delta_up=0,
  delta_down=0,
  rate_up=0,
  rate_down=0,
  window_seconds=0,
  status=excluded.status
`)
	if err != nil {
		return err
	}
	defer insertSample.Close()
	for _, marker := range normalizedStats {
		current := currentStates[trafficStateKey(marker.Engine, marker.ScopeType, marker.ScopeKey)]
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_states (engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, delta_up, delta_down, rate_up, rate_down, window_seconds, last_seen_at, status, message)
VALUES (?, ?, ?, 0, 0, 0, 0, 0, 0, 0, 0, 0, ?, ?, ?)
ON CONFLICT(engine, scope_type, scope_key) DO UPDATE SET
  delta_up=0,
  delta_down=0,
  rate_up=0,
  rate_down=0,
  window_seconds=0,
  last_seen_at=excluded.last_seen_at,
  status=excluded.status,
  message=excluded.message
`, marker.Engine, marker.ScopeType, marker.ScopeKey, seenAt, marker.Status, marker.Message); err != nil {
			return err
		}
		if _, err := insertSample.ExecContext(ctx, sampleBucketAt, marker.Engine, marker.ScopeType, marker.ScopeKey, current.TotalUp, current.TotalDown, marker.Status); err != nil {
			return err
		}
	}
	rollbackCleanup, err := s.cleanupTrafficSamples(ctx, tx, observedAt)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		rollbackCleanup()
		return err
	}
	return nil
}

func normalizeTrafficStatusMarkers(stats []TrafficStatusMarker) []TrafficStatusMarker {
	normalized := make([]TrafficStatusMarker, 0, len(stats))
	for _, marker := range stats {
		marker.Engine = normalizeTrafficToken(marker.Engine)
		marker.ScopeType = normalizeTrafficToken(marker.ScopeType)
		marker.ScopeKey = strings.TrimSpace(marker.ScopeKey)
		marker.Status = strings.TrimSpace(marker.Status)
		marker.Message = strings.TrimSpace(marker.Message)
		if marker.Status == "" {
			marker.Status = "unavailable"
		}
		if marker.Engine == "" || marker.ScopeType == "" || marker.ScopeKey == "" {
			continue
		}
		normalized = append(normalized, marker)
	}
	return normalized
}

func trafficRawStatsForStatusMarkers(markers []TrafficStatusMarker) []TrafficRawStat {
	rawStats := make([]TrafficRawStat, 0, len(markers))
	for _, marker := range markers {
		rawStats = append(rawStats, TrafficRawStat{Engine: marker.Engine, ScopeType: marker.ScopeType, ScopeKey: marker.ScopeKey})
	}
	return rawStats
}

func (s *Store) ResetClientTrafficBaseline(ctx context.Context, id int64, baselines []TrafficRawStat) (Client, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, traffic_limit, expiry_at FROM clients WHERE id=?`, id)
	var client Client
	var dbEnabled int
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	now := time.Now().UTC().Format(time.RFC3339Nano)
	existingEngines, err := trafficStateEnginesForClient(ctx, tx, client.StatsKey)
	if err != nil {
		return Client{}, err
	}
	expectedEngine, ok, err := clientExpectedTrafficEngineByID(ctx, tx, client.ID)
	if err != nil {
		return Client{}, err
	}
	if ok && expectedEngine != "" {
		existingEngines[expectedEngine] = struct{}{}
	}
	baselineByEngine := map[string]TrafficRawStat{}
	for _, raw := range baselines {
		if normalizeTrafficToken(raw.ScopeType) != "client" || strings.TrimSpace(raw.ScopeKey) != client.StatsKey {
			continue
		}
		engine := normalizeTrafficToken(raw.Engine)
		if engine == "" {
			continue
		}
		baselineByEngine[engine] = raw
		existingEngines[engine] = struct{}{}
	}
	for engine := range existingEngines {
		raw, hasBaseline := baselineByEngine[engine]
		status := "waiting"
		message := "baseline reset"
		lastRawUp := int64(0)
		lastRawDown := int64(0)
		if hasBaseline {
			lastRawUp = raw.RawUp
			lastRawDown = raw.RawDown
		} else {
			status = "unavailable"
			message = "baseline unavailable during reset"
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO traffic_states (engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, delta_up, delta_down, rate_up, rate_down, window_seconds, last_seen_at, status, message)
VALUES (?, 'client', ?, 0, 0, ?, ?, 0, 0, 0, 0, 0, ?, ?, ?)
ON CONFLICT(engine, scope_type, scope_key) DO UPDATE SET
  total_up=0,
  total_down=0,
  last_raw_up=excluded.last_raw_up,
  last_raw_down=excluded.last_raw_down,
  delta_up=0,
  delta_down=0,
  rate_up=0,
  rate_down=0,
  window_seconds=0,
  last_seen_at=excluded.last_seen_at,
  status=excluded.status,
  message=excluded.message
`, engine, client.StatsKey, lastRawUp, lastRawDown, now, status, message); err != nil {
			return Client{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	return client, nil
}

func (s *Store) ListTrafficStates(ctx context.Context) ([]TrafficState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, delta_up, delta_down, rate_up, rate_down, window_seconds, last_seen_at, status, message FROM traffic_states ORDER BY engine, scope_type, scope_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []TrafficState{}
	for rows.Next() {
		var state TrafficState
		if err := rows.Scan(&state.Engine, &state.ScopeType, &state.ScopeKey, &state.TotalUp, &state.TotalDown, &state.LastRawUp, &state.LastRawDown, &state.DeltaUp, &state.DeltaDown, &state.RateUp, &state.RateDown, &state.WindowSeconds, &state.LastSeenAt, &state.Status, &state.Message); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (s *Store) ListTrafficSamples(ctx context.Context, scopeType string, since time.Time, limit int) ([]TrafficSample, error) {
	if limit <= 0 || limit > 2000 {
		limit = 2000
	}
	return s.ListTrafficSamplesWindow(ctx, scopeType, since, time.Time{}, limit)
}

func (s *Store) ListTrafficSamplesWindow(ctx context.Context, scopeType string, since time.Time, until time.Time, limit int) ([]TrafficSample, error) {
	scopeType = normalizeTrafficToken(scopeType)
	if scopeType == "" {
		scopeType = "core"
	}
	sinceText := since.UTC().Format(time.RFC3339Nano)
	where := `scope_type = ? AND sampled_at >= ?`
	args := []interface{}{scopeType, sinceText}
	if !until.IsZero() {
		where += ` AND sampled_at <= ?`
		args = append(args, until.UTC().Format(time.RFC3339Nano))
	}
	query := `
SELECT sampled_at, engine, scope_type, scope_key, total_up, total_down, delta_up, delta_down, rate_up, rate_down, window_seconds, status
FROM traffic_samples
WHERE ` + where + `
ORDER BY sampled_at ASC, engine ASC, scope_key ASC`
	if limit > 0 {
		query = `
SELECT sampled_at, engine, scope_type, scope_key, total_up, total_down, delta_up, delta_down, rate_up, rate_down, window_seconds, status
FROM (
  SELECT sampled_at, engine, scope_type, scope_key, total_up, total_down, delta_up, delta_down, rate_up, rate_down, window_seconds, status
  FROM traffic_samples
  WHERE ` + where + `
  ORDER BY sampled_at DESC, engine DESC, scope_key DESC
  LIMIT ?
)
ORDER BY sampled_at ASC, engine ASC, scope_key ASC`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples := []TrafficSample{}
	for rows.Next() {
		var sample TrafficSample
		if err := rows.Scan(&sample.SampledAt, &sample.Engine, &sample.ScopeType, &sample.ScopeKey, &sample.TotalUp, &sample.TotalDown, &sample.DeltaUp, &sample.DeltaDown, &sample.RateUp, &sample.RateDown, &sample.WindowSeconds, &sample.Status); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *Store) ListTrafficAnalyticsSamples(ctx context.Context, params TrafficAnalyticsSampleParams) ([]TrafficSample, error) {
	scopeType := normalizeTrafficToken(params.ScopeType)
	if scopeType == "" {
		scopeType = "core"
	}
	bucketSeconds := params.BucketSeconds
	if bucketSeconds <= 0 {
		bucketSeconds = 300
	}
	if bucketSeconds > 86400 {
		bucketSeconds = 86400
	}
	sinceText := params.Since.UTC().Format(time.RFC3339Nano)
	where := `scope_type = ? AND sampled_at >= ?`
	args := []interface{}{scopeType, sinceText}
	if !params.Until.IsZero() {
		where += ` AND sampled_at <= ?`
		args = append(args, params.Until.UTC().Format(time.RFC3339Nano))
	}
	query := fmt.Sprintf(`
WITH bucketed AS (
  SELECT
    ((CAST(strftime('%%s', sampled_at) AS INTEGER) / %[1]d) * %[1]d) AS bucket_unix,
    sampled_at,
    engine,
    scope_type,
    scope_key,
    total_up,
    total_down,
    delta_up,
    delta_down,
    rate_up,
    rate_down,
    window_seconds,
    status,
    ROW_NUMBER() OVER (
      PARTITION BY ((CAST(strftime('%%s', sampled_at) AS INTEGER) / %[1]d) * %[1]d), engine, scope_type, scope_key
      ORDER BY sampled_at DESC
    ) AS rn
  FROM traffic_samples
  WHERE `+where+`
)
SELECT
  strftime('%%Y-%%m-%%dT%%H:%%M:%%SZ', bucket_unix, 'unixepoch') AS sampled_at,
  engine,
  scope_type,
  scope_key,
  MAX(CASE WHEN rn = 1 THEN total_up ELSE 0 END) AS total_up,
  MAX(CASE WHEN rn = 1 THEN total_down ELSE 0 END) AS total_down,
  SUM(delta_up) AS delta_up,
  SUM(delta_down) AS delta_down,
  AVG(rate_up) AS rate_up,
  AVG(rate_down) AS rate_down,
  MAX(window_seconds) AS window_seconds,
  CASE
    WHEN SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END) = COUNT(*) THEN 'ok'
    WHEN SUM(CASE WHEN status = 'not_configured' THEN 1 ELSE 0 END) = COUNT(*) THEN 'not_configured'
    WHEN COUNT(DISTINCT status) > 1 THEN 'partial'
    WHEN SUM(CASE WHEN status = 'partial' THEN 1 ELSE 0 END) > 0 THEN 'partial'
    WHEN SUM(CASE WHEN status = 'unsupported' THEN 1 ELSE 0 END) > 0 AND SUM(CASE WHEN status IN ('unavailable', 'stale') THEN 1 ELSE 0 END) = 0 THEN 'unsupported'
    WHEN SUM(CASE WHEN status = 'stale' THEN 1 ELSE 0 END) > 0 AND SUM(CASE WHEN status = 'unavailable' THEN 1 ELSE 0 END) = 0 THEN 'stale'
    WHEN SUM(CASE WHEN status = 'unavailable' THEN 1 ELSE 0 END) > 0 THEN 'unavailable'
    ELSE 'waiting'
  END AS status
FROM bucketed
GROUP BY bucket_unix, engine, scope_type, scope_key
ORDER BY bucket_unix ASC, engine ASC, scope_key ASC`, bucketSeconds)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples := []TrafficSample{}
	for rows.Next() {
		var sample TrafficSample
		if err := rows.Scan(&sample.SampledAt, &sample.Engine, &sample.ScopeType, &sample.ScopeKey, &sample.TotalUp, &sample.TotalDown, &sample.DeltaUp, &sample.DeltaDown, &sample.RateUp, &sample.RateDown, &sample.WindowSeconds, &sample.Status); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (s *Store) GetClientTrafficUsage(ctx context.Context, statsKey string) (ClientTrafficUsage, bool, error) {
	statsKey = strings.TrimSpace(statsKey)
	if statsKey == "" {
		return ClientTrafficUsage{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `
SELECT c.id, c.stats_key, i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.stats_key = ?
LIMIT 1`, statsKey)
	return s.getClientTrafficUsageFromRow(ctx, row)
}

func (s *Store) GetClientTrafficUsageForClient(ctx context.Context, clientID int64) (ClientTrafficUsage, bool, error) {
	if clientID <= 0 {
		return ClientTrafficUsage{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `
SELECT c.id, c.stats_key, i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.id = ?
LIMIT 1`, clientID)
	return s.getClientTrafficUsageFromRow(ctx, row)
}

func (s *Store) getClientTrafficUsageFromRow(ctx context.Context, row *sql.Row) (ClientTrafficUsage, bool, error) {
	var clientID int64
	var statsKey string
	var protocol string
	if err := row.Scan(&clientID, &statsKey, &protocol); err != nil {
		if err == sql.ErrNoRows {
			return ClientTrafficUsage{}, false, nil
		}
		return ClientTrafficUsage{}, false, err
	}
	states, err := s.trafficStatesForClient(ctx, statsKey)
	if err != nil {
		return ClientTrafficUsage{}, false, err
	}
	usage := chooseClientTrafficUsage(clientID, statsKey, expectedTrafficEngineForProtocol(protocol), states)
	return usage, true, nil
}

func (s *Store) trafficStatesForClient(ctx context.Context, statsKey string) ([]TrafficState, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT engine, scope_type, scope_key, total_up, total_down, last_raw_up, last_raw_down, delta_up, delta_down, rate_up, rate_down, window_seconds, last_seen_at, status, message
FROM traffic_states
WHERE scope_type='client' AND scope_key=?
ORDER BY engine ASC`, statsKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := []TrafficState{}
	for rows.Next() {
		var state TrafficState
		if err := rows.Scan(&state.Engine, &state.ScopeType, &state.ScopeKey, &state.TotalUp, &state.TotalDown, &state.LastRawUp, &state.LastRawDown, &state.DeltaUp, &state.DeltaDown, &state.RateUp, &state.RateDown, &state.WindowSeconds, &state.LastSeenAt, &state.Status, &state.Message); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func chooseClientTrafficUsage(clientID int64, statsKey, expectedEngine string, states []TrafficState) ClientTrafficUsage {
	byEngine := map[string]TrafficState{}
	for _, state := range states {
		if normalizeTrafficToken(state.ScopeType) != "client" || strings.TrimSpace(state.ScopeKey) != statsKey {
			continue
		}
		engine := normalizeTrafficEngine(state.Engine)
		if engine == "" {
			continue
		}
		state.Engine = engine
		state.Status = normalizeTrafficStatus(state.Status)
		byEngine[engine] = state
	}
	expectedEngine = normalizeTrafficEngine(expectedEngine)
	if state, ok := byEngine[expectedEngine]; ok {
		return usageFromTrafficState(clientID, statsKey, state)
	}
	return ClientTrafficUsage{ClientID: clientID, StatsKey: statsKey, Engine: expectedEngine, Status: "waiting"}
}

func usageFromTrafficState(clientID int64, statsKey string, state TrafficState) ClientTrafficUsage {
	return ClientTrafficUsage{
		ClientID: clientID, StatsKey: statsKey, Engine: normalizeTrafficEngine(state.Engine),
		TotalUp: state.TotalUp, TotalDown: state.TotalDown, RateUp: state.RateUp, RateDown: state.RateDown,
		Status: normalizeTrafficStatus(state.Status), Message: state.Message, LastSeenAt: state.LastSeenAt,
	}
}

func trafficStateEnginesForClient(ctx context.Context, tx *sql.Tx, statsKey string) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `SELECT engine FROM traffic_states WHERE scope_type='client' AND scope_key=? ORDER BY engine ASC`, statsKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	engines := map[string]struct{}{}
	for rows.Next() {
		var engine string
		if err := rows.Scan(&engine); err != nil {
			return nil, err
		}
		if engine = normalizeTrafficEngine(engine); engine != "" {
			engines[engine] = struct{}{}
		}
	}
	return engines, rows.Err()
}

func clientExpectedTrafficEngineByID(ctx context.Context, tx *sql.Tx, clientID int64) (string, bool, error) {
	var protocol string
	if err := tx.QueryRowContext(ctx, `
SELECT i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.id = ?
LIMIT 1`, clientID).Scan(&protocol); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return expectedTrafficEngineForProtocol(protocol), true, nil
}

func lookupClientExpectedTrafficEngine(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, statsKey string) (string, bool, error) {
	var protocol string
	if err := querier.QueryRowContext(ctx, `
SELECT i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.stats_key = ?
LIMIT 1`, statsKey).Scan(&protocol); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return expectedTrafficEngineForProtocol(protocol), true, nil
}

func lookupClientTrafficIdentity(ctx context.Context, querier interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, statsKey string) (string, string, error) {
	var matchedStatsKey string
	var protocol string
	if err := querier.QueryRowContext(ctx, `
SELECT c.stats_key, i.protocol
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
WHERE c.stats_key = ?
LIMIT 1`, statsKey).Scan(&matchedStatsKey, &protocol); err != nil {
		return "", "", err
	}
	engine := expectedTrafficEngineForProtocol(protocol)
	if strings.TrimSpace(engine) == "" {
		engine = "xray"
	}
	return matchedStatsKey, engine, nil
}

func expectedTrafficEngineForProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "hysteria2", "tuic", "shadowtls":
		return "singbox"
	default:
		return "xray"
	}
}

func normalizeTrafficEngine(engine string) string {
	switch normalizeTrafficToken(engine) {
	case "sing-box":
		return "singbox"
	default:
		return normalizeTrafficToken(engine)
	}
}

func normalizeTrafficStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "waiting"
	}
	return status
}

func normalizeTrafficToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
