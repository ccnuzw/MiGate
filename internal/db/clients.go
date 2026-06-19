package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CreateClient(ctx context.Context, params CreateClientParams) (Client, error) {
	if params.InboundID <= 0 {
		return Client{}, fmt.Errorf("invalid inbound id: %d", params.InboundID)
	}
	inbound, err := s.getInboundBasic(ctx, params.InboundID)
	if err != nil {
		return Client{}, err
	}
	client, err := s.prepareClientForCreate(ctx, inbound, params)
	if err != nil {
		return Client{}, err
	}
	return s.insertClientTx(ctx, s.db, client)
}

func (s *Store) prepareClientForCreate(ctx context.Context, inbound Inbound, params CreateClientParams) (Client, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" {
		email = "client"
	}
	uuid, credentialID, password := normalizeClientCredentials(inbound.Protocol, params.UUID, params.CredentialID, params.Password)
	clientForValidation := Client{UUID: uuid, CredentialID: credentialID, Password: password}
	if err := ValidateClientCredential(inbound.Protocol, clientForValidation); err != nil {
		return Client{}, err
	}
	var existingID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE inbound_id = ? AND email = ? LIMIT 1`, params.InboundID, email).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client email: %s", email)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE uuid = ? LIMIT 1`, uuid).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client uuid: %s", uuid)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if credentialID != "" {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE credential_id = ? LIMIT 1`, credentialID).Scan(&existingID); err == nil {
			return Client{}, fmt.Errorf("duplicate client credential_id: %s", credentialID)
		} else if err != sql.ErrNoRows {
			return Client{}, err
		}
	}
	subscriptionToken, err := s.newSubscriptionToken(ctx)
	if err != nil {
		return Client{}, err
	}
	statsKey, err := s.newStatsKey(ctx)
	if err != nil {
		return Client{}, err
	}
	enabled := true
	if params.Enabled != nil {
		enabled = *params.Enabled
	}
	return Client{InboundID: params.InboundID, UUID: uuid, CredentialID: credentialID, Password: password, SubscriptionToken: subscriptionToken, StatsKey: statsKey, Email: email, Enabled: enabled, TrafficLimit: params.TrafficLimit, ExpiryAt: params.ExpiryAt}, nil
}

func (s *Store) insertClientTx(ctx context.Context, execer sqlExecer, client Client) (Client, error) {
	dbEnabled := 0
	if client.Enabled {
		dbEnabled = 1
	}
	result, err := execer.ExecContext(ctx, `
INSERT INTO clients (inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, created_at, traffic_limit, expiry_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, client.InboundID, client.UUID, client.CredentialID, client.Password, client.SubscriptionToken, client.StatsKey, client.Email, dbEnabled, time.Now().UTC().Format(time.RFC3339), client.TrafficLimit, client.ExpiryAt)
	if err != nil {
		return Client{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Client{}, err
	}
	client.ID = id
	return client, nil
}

func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM routing_rules WHERE client_id = ?`, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("client not found: %d", id)
	}
	return tx.Commit()
}
func normalizeClientCredentials(protocol, uuid, credentialID, password string) (string, string, string) {
	protocol = NormalizeInboundProtocol(protocol)
	uuid = strings.TrimSpace(uuid)
	credentialID = strings.TrimSpace(credentialID)
	password = strings.TrimSpace(password)
	if credentialID == "" {
		credentialID = uuid
	}
	capability, _ := GetInboundCapability(protocol)
	switch capability.CredentialType {
	case CredentialUUID:
		if credentialID == "" {
			credentialID = newUUID()
		}
		return credentialID, credentialID, ""
	case CredentialPassword:
		if password == "" {
			password = uuid
		}
		if password == "" {
			password = newSecret(24)
		}
		if credentialID == "" {
			credentialID = newUUID()
		}
		return password, credentialID, password
	case CredentialIDPassword:
		if credentialID == "" {
			credentialID = newUUID()
		}
		if password == "" {
			password = newSecret(24)
		}
		return credentialID, credentialID, password
	case CredentialUsernamePassword:
		if credentialID == "" {
			credentialID = "user-" + newSecret(8)
		}
		if password == "" {
			password = newSecret(24)
		}
		return credentialID, credentialID, password
	case CredentialNone:
		if uuid == "" {
			uuid = newSecret(24)
		}
		return uuid, uuid, ""
	default:
		if uuid == "" {
			uuid = newUUID()
		}
		return uuid, uuid, password
	}
}
func (s *Store) UpdateClient(ctx context.Context, id int64, params UpdateClientParams) (Client, error) {
	email := strings.TrimSpace(params.Email)
	if email == "" {
		email = "client"
	}
	enabled := 0
	if params.Enabled {
		enabled = 1
	}
	var inboundID int64
	var existingUUID string
	var existingCredentialID string
	var existingPassword string
	var oldEmail string
	if err := s.db.QueryRowContext(ctx, `SELECT inbound_id, uuid, credential_id, password, email FROM clients WHERE id = ?`, id).Scan(&inboundID, &existingUUID, &existingCredentialID, &existingPassword, &oldEmail); err != nil {
		return Client{}, err
	}
	inbound, err := s.getInboundBasic(ctx, inboundID)
	if err != nil {
		return Client{}, err
	}
	rawUUID := firstNonEmpty(params.UUID, existingUUID)
	rawCredentialID := firstNonEmpty(params.CredentialID, existingCredentialID, rawUUID)
	rawPassword := firstNonEmpty(params.Password, existingPassword)
	if capability, ok := GetInboundCapability(inbound.Protocol); ok {
		switch capability.CredentialType {
		case CredentialUUID:
			rawCredentialID = rawUUID
		case CredentialPassword:
			if strings.TrimSpace(params.Password) == "" && strings.TrimSpace(params.UUID) != "" {
				rawPassword = params.UUID
			}
		case CredentialIDPassword, CredentialUsernamePassword:
			if strings.TrimSpace(params.CredentialID) == "" && strings.TrimSpace(params.UUID) != "" {
				rawCredentialID = params.UUID
			}
		}
	}
	uuid, credentialID, password := normalizeClientCredentials(inbound.Protocol, rawUUID, rawCredentialID, rawPassword)
	if err := ValidateClientCredential(inbound.Protocol, Client{UUID: uuid, CredentialID: credentialID, Password: password}); err != nil {
		return Client{}, err
	}
	var existingID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE inbound_id = ? AND email = ? AND id <> ? LIMIT 1`, inboundID, email, id).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client email: %s", email)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE uuid = ? AND id <> ? LIMIT 1`, uuid, id).Scan(&existingID); err == nil {
		return Client{}, fmt.Errorf("duplicate client uuid: %s", uuid)
	} else if err != sql.ErrNoRows {
		return Client{}, err
	}
	if credentialID != "" {
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE credential_id = ? AND id <> ? LIMIT 1`, credentialID, id).Scan(&existingID); err == nil {
			return Client{}, fmt.Errorf("duplicate client credential_id: %s", credentialID)
		} else if err != sql.ErrNoRows {
			return Client{}, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE clients SET uuid=?, credential_id=?, password=?, email=?, enabled=?, traffic_limit=?, expiry_at=? WHERE id=?`,
		uuid, credentialID, password, email, enabled, params.TrafficLimit, params.ExpiryAt, id)
	if err != nil {
		return Client{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Client{}, err
	}
	if n == 0 {
		return Client{}, fmt.Errorf("client not found: %d", id)
	}
	if strings.TrimSpace(oldEmail) != email {
		if _, err := tx.ExecContext(ctx, `UPDATE routing_rules SET client_email=? WHERE client_id=?`, email, id); err != nil {
			return Client{}, err
		}
	}
	row := tx.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at FROM clients WHERE id=?`, id)
	var client Client
	var dbEnabled int
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	return client, nil
}
func (s *Store) SetClientEnabled(ctx context.Context, inboundID int64, id int64, enabled bool) (Client, error) {
	dbEnabled := 0
	if enabled {
		dbEnabled = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE clients SET enabled=? WHERE inbound_id=? AND id=?`, dbEnabled, inboundID, id)
	if err != nil {
		return Client{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Client{}, err
	}
	if n == 0 {
		return Client{}, fmt.Errorf("client not found: %d", id)
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, inbound_id, uuid, credential_id, password, subscription_token, stats_key, email, enabled, up, down, traffic_limit, expiry_at FROM clients WHERE inbound_id=? AND id=?`, inboundID, id)
	var client Client
	if err := row.Scan(&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &dbEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		return Client{}, err
	}
	client.Enabled = dbEnabled != 0
	return client, nil
}

func (s *Store) GetSubscriptionByClientUUID(ctx context.Context, uuid string) (Inbound, Client, bool, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return Inbound{}, Client{}, false, nil
	}
	return s.getSubscriptionByClientRow(s.db.QueryRowContext(ctx, subscriptionLookupSQLByUUID, uuid))
}

func (s *Store) GetSubscriptionByToken(ctx context.Context, token string) (Inbound, Client, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Inbound{}, Client{}, false, nil
	}
	return s.getSubscriptionByClientRow(s.db.QueryRowContext(ctx, subscriptionLookupSQLByToken, token))
}

const subscriptionLookupSelect = `
SELECT i.id, i.uuid, i.remark, i.protocol, i.core, i.port, i.network, i.security, i.enabled,
  i.ws_path, i.ws_host, i.grpc_service_name, i.reality_dest, i.reality_server_names, i.reality_short_id, i.reality_private_key, i.reality_public_key, i.ss_method,
  i.tls_cert_file, i.tls_key_file, i.tls_sni, i.tls_fingerprint, i.tls_alpn, i.xhttp_path, i.xhttp_mode,
  i.hy2_up_mbps, i.hy2_down_mbps, i.hy2_obfs, i.hy2_obfs_password, i.hy2_mport,
  i.tuic_congestion_control, i.tuic_zero_rtt,
  i.wg_private_key, i.wg_address, i.wg_peer_public_key, i.wg_allowed_ips, i.wg_endpoint, i.wg_preshared_key, i.wg_mtu,
  i.shadowtls_version, i.shadowtls_password,
  c.id, c.inbound_id, c.uuid, c.credential_id, c.password, c.subscription_token, c.stats_key, c.email, c.enabled, c.up, c.down, c.traffic_limit, c.expiry_at
FROM clients c
JOIN inbounds i ON i.id = c.inbound_id
`

const subscriptionLookupSQLByUUID = subscriptionLookupSelect + `
WHERE c.uuid = ?
LIMIT 1
`

const subscriptionLookupSQLByToken = subscriptionLookupSelect + `
WHERE c.subscription_token = ?
LIMIT 1
`

func (s *Store) getSubscriptionByClientRow(row *sql.Row) (Inbound, Client, bool, error) {
	var inbound Inbound
	var client Client
	var inboundEnabled int
	var clientEnabled int
	if err := row.Scan(&inbound.ID, &inbound.UUID, &inbound.Remark, &inbound.Protocol, &inbound.Core, &inbound.Port, &inbound.Network, &inbound.Security, &inboundEnabled,
		&inbound.WsPath, &inbound.WsHost, &inbound.GrpcServiceName, &inbound.RealityDest, &inbound.RealityServerNames, &inbound.RealityShortID, &inbound.RealityPrivateKey, &inbound.RealityPublicKey, &inbound.SSMethod,
		&inbound.TLSCertFile, &inbound.TLSKeyFile, &inbound.TLSSNI, &inbound.TLSFingerprint, &inbound.TLSALPN, &inbound.XHTTPPath, &inbound.XHTTPMode,
		&inbound.Hy2UpMbps, &inbound.Hy2DownMbps, &inbound.Hy2Obfs, &inbound.Hy2ObfsPassword, &inbound.Hy2MPort,
		&inbound.TuicCongestionControl, &inbound.TuicZeroRTT,
		&inbound.WgPrivateKey, &inbound.WgAddress, &inbound.WgPeerPublicKey, &inbound.WgAllowedIPs, &inbound.WgEndpoint, &inbound.WgPresharedKey, &inbound.WgMTU,
		&inbound.ShadowTLSVersion, &inbound.ShadowTLSPassword,
		&client.ID, &client.InboundID, &client.UUID, &client.CredentialID, &client.Password, &client.SubscriptionToken, &client.StatsKey, &client.Email, &clientEnabled, &client.Up, &client.Down, &client.TrafficLimit, &client.ExpiryAt); err != nil {
		if err == sql.ErrNoRows {
			return Inbound{}, Client{}, false, nil
		}
		return Inbound{}, Client{}, false, err
	}
	inbound.Enabled = inboundEnabled != 0
	if inbound.Core == "" {
		inbound.Core = InferInboundCore(inbound.Protocol)
	}
	client.Enabled = clientEnabled != 0
	inbound.Clients = []Client{client}
	return inbound, client, true, nil
}
func (s *Store) newSubscriptionToken(ctx context.Context) (string, error) {
	for i := 0; i < 8; i++ {
		token, err := randomHexToken(24)
		if err != nil {
			return "", err
		}
		var existingID int64
		err = s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE subscription_token = ? LIMIT 1`, token).Scan(&existingID)
		if err == sql.ErrNoRows {
			return token, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("could not generate unique subscription token")
}

func (s *Store) newStatsKey(ctx context.Context) (string, error) {
	for i := 0; i < 8; i++ {
		token, err := randomHexToken(16)
		if err != nil {
			return "", err
		}
		key := "c_" + token
		var existingID int64
		err = s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE stats_key = ? LIMIT 1`, key).Scan(&existingID)
		if err == sql.ErrNoRows {
			return key, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique stats key")
}
