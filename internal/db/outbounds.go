package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

var supportedOutboundProtocols = map[string]bool{
	"freedom":     true,
	"blackhole":   true,
	"dns":         true,
	"socks":       true,
	"http":        true,
	"https":       true,
	"vless":       true,
	"trojan":      true,
	"shadowsocks": true,
	"hysteria2":   true,
	"tuic":        true,
	"shadowtls":   true,
}

func (s *Store) seedDefaultOutbounds(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)
	defaults := []Outbound{
		{Tag: "direct", Remark: "直接连接", Protocol: "freedom", Enabled: true, Sort: 0},
		{Tag: "blocked", Remark: "阻断", Protocol: "blackhole", Enabled: true, Sort: 1},
		{Tag: "dns", Remark: "DNS", Protocol: "dns", Enabled: true, Sort: 2},
	}
	for _, outbound := range defaults {
		_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO outbounds (tag, remark, protocol, address, port, username, password, enabled, sort, created_at) VALUES (?, ?, ?, '', 0, '', '', 1, ?, ?)`,
			outbound.Tag, outbound.Remark, outbound.Protocol, outbound.Sort, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListOutbounds(ctx context.Context) ([]Outbound, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outbounds := []Outbound{}
	for rows.Next() {
		var outbound Outbound
		var enabled int
		if err := rows.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &enabled, &outbound.Sort); err != nil {
			return nil, err
		}
		outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
		outbound.Enabled = enabled != 0
		outbounds = append(outbounds, outbound)
	}
	return outbounds, rows.Err()
}

func (s *Store) CreateOutbound(ctx context.Context, params CreateOutboundParams) (Outbound, error) {
	protocol := NormalizeOutboundProtocol(params.Protocol)
	if !supportedOutboundProtocols[protocol] {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	tag := strings.TrimSpace(params.Tag)
	if tag == "" {
		return Outbound{}, fmt.Errorf("tag cannot be empty")
	}
	remark := strings.TrimSpace(params.Remark)
	if remark == "" {
		remark = tag
	}
	address := strings.TrimSpace(params.Address)
	if outboundProtocolNeedsAddress(protocol) && address == "" {
		return Outbound{}, fmt.Errorf("address cannot be empty")
	}
	if outboundProtocolNeedsAddress(protocol) && (params.Port <= 0 || params.Port > 65535) {
		return Outbound{}, fmt.Errorf("invalid port: %d", params.Port)
	}
	supportedCores := OutboundProtocolSupportedCores(protocol)
	if len(supportedCores) == 0 {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	if err := ValidateOutboundProfile(Outbound{Protocol: protocol, Address: address, Port: params.Port, Username: params.Username, Password: params.Password}); err != nil {
		return Outbound{}, err
	}
	var sort int
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort)+1, 0) FROM outbounds`).Scan(&sort)
	result, err := s.db.ExecContext(ctx, `INSERT INTO outbounds (tag, remark, protocol, address, port, username, password, enabled, sort, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		tag, remark, protocol, address, params.Port, strings.TrimSpace(params.Username), params.Password, sort, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Outbound{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Outbound{}, err
	}
	return Outbound{ID: id, Tag: tag, Remark: remark, Protocol: protocol, Address: address, Port: params.Port, Username: strings.TrimSpace(params.Username), Password: params.Password, SupportedCores: supportedCores, Enabled: true, Sort: sort}, nil
}

func (s *Store) UpdateOutbound(ctx context.Context, id int64, params UpdateOutboundParams) (Outbound, error) {
	protocol := NormalizeOutboundProtocol(params.Protocol)
	if !supportedOutboundProtocols[protocol] {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	tag := strings.TrimSpace(params.Tag)
	if tag == "" {
		return Outbound{}, fmt.Errorf("tag cannot be empty")
	}
	remark := strings.TrimSpace(params.Remark)
	if remark == "" {
		remark = tag
	}
	address := strings.TrimSpace(params.Address)
	if outboundProtocolNeedsAddress(protocol) && address == "" {
		return Outbound{}, fmt.Errorf("address cannot be empty")
	}
	if outboundProtocolNeedsAddress(protocol) && (params.Port <= 0 || params.Port > 65535) {
		return Outbound{}, fmt.Errorf("invalid port: %d", params.Port)
	}
	supportedCores := OutboundProtocolSupportedCores(protocol)
	if len(supportedCores) == 0 {
		return Outbound{}, fmt.Errorf("unsupported outbound protocol: %s", params.Protocol)
	}
	if err := ValidateOutboundProfile(Outbound{Protocol: protocol, Address: address, Port: params.Port, Username: params.Username, Password: params.Password}); err != nil {
		return Outbound{}, err
	}
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE outbounds SET tag=?, remark=?, protocol=?, address=?, port=?, username=?, password=?, enabled=? WHERE id=?`,
		tag, remark, protocol, address, params.Port, strings.TrimSpace(params.Username), params.Password, enabled, id)
	if err != nil {
		return Outbound{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Outbound{}, err
	}
	if n == 0 {
		return Outbound{}, fmt.Errorf("outbound not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds WHERE id=?`, id)
	var outbound Outbound
	var dbEnabled int
	if err := row.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &dbEnabled, &outbound.Sort); err != nil {
		return Outbound{}, err
	}
	outbound.Enabled = dbEnabled != 0
	outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
	return outbound, nil
}

func (s *Store) DeleteOutbound(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM outbounds WHERE id=?`, id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("outbound not found: %d", id)
		}
		return err
	}
	var routeCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM routing_rules WHERE outbound_id=?`, id).Scan(&routeCount); err != nil {
		return err
	}
	if routeCount > 0 {
		return fmt.Errorf("outbound %d is referenced by %d routing rule(s)", id, routeCount)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM outbounds WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("outbound not found: %d", id)
	}
	return tx.Commit()
}

func (s *Store) ReorderOutbounds(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Collect IDs of editable (non-default) outbounds already in DB
	rows, err := tx.QueryContext(ctx, `SELECT id FROM outbounds WHERE protocol NOT IN ('freedom','blackhole','dns') ORDER BY sort ASC`)
	if err != nil {
		return err
	}
	var existing []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing = append(existing, id)
	}
	rows.Close()

	if len(ids) != len(existing) {
		return fmt.Errorf("expected %d IDs for reordering, got %d", len(existing), len(ids))
	}

	// Find defaults count
	var defaultCount int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM outbounds WHERE protocol IN ('freedom','blackhole','dns')`).Scan(&defaultCount); err != nil {
		return err
	}

	for i, id := range ids {
		_, err := tx.ExecContext(ctx, `UPDATE outbounds SET sort = ? WHERE id = ?`, int(defaultCount)+i, id)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetOutboundEnabled(ctx context.Context, id int64, enabled bool) (Outbound, error) {
	dbEnabled := 0
	if enabled {
		dbEnabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE outbounds SET enabled=? WHERE id=?`, dbEnabled, id)
	if err != nil {
		return Outbound{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Outbound{}, err
	}
	if n == 0 {
		return Outbound{}, fmt.Errorf("outbound not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, tag, remark, protocol, address, port, username, password, enabled, sort FROM outbounds WHERE id=?`, id)
	var outbound Outbound
	var dbEnabledInt int
	if err := row.Scan(&outbound.ID, &outbound.Tag, &outbound.Remark, &outbound.Protocol, &outbound.Address, &outbound.Port, &outbound.Username, &outbound.Password, &dbEnabledInt, &outbound.Sort); err != nil {
		return Outbound{}, err
	}
	outbound.Enabled = dbEnabledInt != 0
	outbound.SupportedCores = OutboundProtocolSupportedCores(outbound.Protocol)
	return outbound, nil
}
