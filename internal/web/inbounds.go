package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/singbox"
	"github.com/imzyb/MiGate/internal/xray"
)

func inboundsHandler(cfg *routerConfig) http.HandlerFunc {
	store, ctrl, statsClient := cfg.store, cfg.xrayController, cfg.statsClient
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listInbounds(w, r, store, statsClient)
		case http.MethodPost:
			created, ok := createInbound(w, r, store)
			if ok {
				if db.InboundCore(created) == db.CoreSingbox {
					applyXrayAsync(ctrl)
					writeCreatedSingboxResult(w, cfg, r, store, map[string]interface{}{"inbound": created})
					return
				}
				applyCoreAsync(ctrl, store)
			}
		default:
			methodNotAllowed(w)
		}
	}
}

func inboundCapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"capabilities": db.InboundCapabilities()})
}

func realityKeypairHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	privateKey, publicKey, err := xray.GenerateRealityKey()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "generate_reality_keypair_failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"private_key": privateKey, "public_key": publicKey})
}

func applyCoreAsync(ctrl XrayController, store Store) {
	applyXrayAsync(ctrl)
	applySingboxAsync(store)
}

func applyXrayAsync(ctrl XrayController) {
	if ctrl == nil {
		ctrl = defaultXrayController{}
	}
	go func() {
		result := ctrl.Apply(context.Background())
		if strings.HasPrefix(result.Status, "failed") {
			log.Printf("xray apply failed: status=%s service=%s commands=%v error=%s", result.Status, result.Service, result.CommandsExecuted, result.ErrorOutput)
		}
	}()
}

func applySingboxAsync(store Store) {
	if store == nil {
		return
	}
	go func() {
		if err := tryApplySingbox(context.Background(), store); err != nil {
			log.Printf("sing-box auto apply: %v", err)
		}
	}()
}

func strictSingboxApply(ctx context.Context, cfg *routerConfig, store Store) error {
	if cfg != nil && cfg.singboxApplier != nil {
		return cfg.singboxApplier(ctx, store, cfg.singboxRuntime, true)
	}
	return tryApplySingboxWithRuntime(ctx, store, defaultSingboxRuntime{}, true)
}

func writeCreatedSingboxResult(w http.ResponseWriter, cfg *routerConfig, r *http.Request, store Store, payload map[string]interface{}) {
	if err := strictSingboxApply(r.Context(), cfg, store); err != nil {
		code := "singbox_apply_failed"
		if errors.Is(err, errSingboxNotInstalled) || err.Error() == errSingboxNotInstalled.Error() {
			code = "singbox_not_installed"
		}
		payload["created"] = true
		payload["applied"] = false
		payload["error"] = code
		payload["detail"] = err.Error()
		payload["singbox"] = map[string]interface{}{"applied": false, "error": code, "detail": err.Error()}
		writeJSON(w, http.StatusCreated, payload)
		return
	}
	payload["created"] = true
	payload["applied"] = true
	payload["singbox"] = map[string]interface{}{"applied": true}
	writeJSON(w, http.StatusCreated, payload)
}

func deriveRealityPublicKeys(inbounds []db.Inbound) {
	for i := range inbounds {
		if inbounds[i].Security == "reality" && inbounds[i].RealityPublicKey == "" && inbounds[i].RealityPrivateKey != "" {
			if pubKey, err := xray.DeriveRealityPublicKey(inbounds[i].RealityPrivateKey); err == nil {
				inbounds[i].RealityPublicKey = pubKey
			}
		}
	}
}

func prepareInboundRealityKeys(payload *db.CreateInboundParams) error {
	if strings.ToLower(strings.TrimSpace(payload.Security)) != "reality" {
		return nil
	}
	if strings.TrimSpace(payload.RealityPrivateKey) == "" {
		privKey, pubKey, err := xray.GenerateRealityKey()
		if err != nil {
			return err
		}
		payload.RealityPrivateKey = privKey
		payload.RealityPublicKey = pubKey
		return ensureRealityShortID(&payload.RealityShortID)
	}
	if strings.TrimSpace(payload.RealityPublicKey) == "" {
		pubKey, err := xray.DeriveRealityPublicKey(payload.RealityPrivateKey)
		if err != nil {
			return err
		}
		payload.RealityPublicKey = pubKey
	}
	return ensureRealityShortID(&payload.RealityShortID)
}

func prepareUpdateInboundRealityKeys(ctx context.Context, store Store, inboundID int64, payload *db.UpdateInboundParams) error {
	if strings.ToLower(strings.TrimSpace(payload.Security)) != "reality" {
		return nil
	}
	if store != nil && (strings.TrimSpace(payload.RealityPrivateKey) == "" || strings.TrimSpace(payload.RealityPublicKey) == "" || strings.TrimSpace(payload.RealityShortID) == "") {
		if inbounds, err := store.ListInbounds(ctx); err == nil {
			for _, inbound := range inbounds {
				if inbound.ID != inboundID {
					continue
				}
				if strings.TrimSpace(payload.RealityPrivateKey) == "" {
					payload.RealityPrivateKey = inbound.RealityPrivateKey
				}
				if strings.TrimSpace(payload.RealityPublicKey) == "" {
					payload.RealityPublicKey = inbound.RealityPublicKey
				}
				if strings.TrimSpace(payload.RealityShortID) == "" {
					payload.RealityShortID = inbound.RealityShortID
				}
				break
			}
		}
	}
	if strings.TrimSpace(payload.RealityPrivateKey) == "" {
		privKey, pubKey, err := xray.GenerateRealityKey()
		if err != nil {
			return err
		}
		payload.RealityPrivateKey = privKey
		payload.RealityPublicKey = pubKey
		return ensureRealityShortID(&payload.RealityShortID)
	}
	if strings.TrimSpace(payload.RealityPublicKey) == "" {
		pubKey, err := xray.DeriveRealityPublicKey(payload.RealityPrivateKey)
		if err != nil {
			return err
		}
		payload.RealityPublicKey = pubKey
	}
	return ensureRealityShortID(&payload.RealityShortID)
}

func ensureRealityShortID(value *string) error {
	if strings.TrimSpace(*value) != "" {
		return nil
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	*value = hex.EncodeToString(b[:])
	return nil
}

func listInbounds(w http.ResponseWriter, r *http.Request, store Store, statsClient xray.StatsClient) {
	inbounds := []db.Inbound{}
	refreshTraffic := r.URL.Query().Get("refresh") == "traffic"
	if store != nil {
		var loaded []db.Inbound
		var err error
		if refreshTraffic {
			loaded, err = store.ListInboundTraffic(r.Context())
		} else {
			loaded, err = store.ListInbounds(r.Context())
			if err == nil {
				deriveRealityPublicKeys(loaded)
			}
		}
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
			return
		}
		inbounds = loaded
	}
	trafficByInbound, trafficByClient := summarizeTraffic(r.Context(), store, inbounds)
	if refreshTraffic {
		views := make([]inboundTrafficView, 0, len(inbounds))
		for _, inbound := range inbounds {
			summary := trafficByInbound[inbound.ID]
			view := inboundTrafficView{
				ID:             inbound.ID,
				UUID:           inbound.UUID,
				Remark:         inbound.Remark,
				Protocol:       inbound.Protocol,
				Port:           inbound.Port,
				Network:        inbound.Network,
				Security:       inbound.Security,
				Enabled:        inbound.Enabled,
				Clients:        inbound.Clients,
				TrafficUp:      summary.Up,
				TrafficDown:    summary.Down,
				TrafficTotal:   summary.Total,
				RateUp:         summary.RateUp,
				RateDown:       summary.RateDown,
				TrafficStatus:  summary.Status,
				TrafficMessage: summary.Message,
				TrafficSource:  summary.Source,
				ClientTraffic:  map[int64]clientTrafficSummary{},
			}
			for _, client := range inbound.Clients {
				if clientTraffic, ok := trafficByClient[client.ID]; ok {
					view.ClientTraffic[client.ID] = clientTraffic
				}
			}
			views = append(views, view)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"inbounds": views})
		return
	}
	views := make([]inboundView, 0, len(inbounds))
	for _, inbound := range inbounds {
		summary := trafficByInbound[inbound.ID]
		view := inboundView{
			Inbound:        inbound,
			TrafficUp:      summary.Up,
			TrafficDown:    summary.Down,
			TrafficTotal:   summary.Total,
			RateUp:         summary.RateUp,
			RateDown:       summary.RateDown,
			TrafficStatus:  summary.Status,
			TrafficMessage: summary.Message,
			TrafficSource:  summary.Source,
			ClientTraffic:  map[int64]clientTrafficSummary{},
		}
		for _, client := range inbound.Clients {
			if clientTraffic, ok := trafficByClient[client.ID]; ok {
				view.ClientTraffic[client.ID] = clientTraffic
			}
		}
		views = append(views, view)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"inbounds": views})
}

func createInbound(w http.ResponseWriter, r *http.Request, store Store) (db.Inbound, bool) {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return db.Inbound{}, false
	}
	var payload db.CreateInboundParams
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return db.Inbound{}, false
	}
	if err := prepareInboundRealityKeys(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "prepare_reality_keys_failed")
		return db.Inbound{}, false
	}
	// Port conflict check
	if payload.Port > 0 {
		conflict, ok, err := store.FindInboundByPort(r.Context(), payload.Port, 0)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "port_check_failed")
			return db.Inbound{}, false
		}
		if ok {
			writeJSONError(w, http.StatusConflict, "port_conflict", map[string]interface{}{
				"message": "端口 " + strconv.FormatInt(int64(conflict.Port), 10) + " 已被入站 " + strconv.FormatInt(conflict.ID, 10) + " 使用",
			})
			return db.Inbound{}, false
		}
	}
	created, err := store.CreateInbound(r.Context(), payload)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return db.Inbound{}, false
	}
	if db.InboundCore(created) != db.CoreSingbox {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	}
	return created, true
}

func inboundChildrenHandler(cfg *routerConfig) http.HandlerFunc {
	store, ctrl, statsClient, singboxStatsClient := cfg.store, cfg.xrayController, cfg.statsClient, cfg.singboxStatsClient
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/inbounds/")
		parts := strings.Split(strings.Trim(path, "/"), "/")

		switch r.Method {
		case http.MethodPost:
			if len(parts) == 4 && parts[1] == "clients" && parts[3] == "reset-traffic" {
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				if resetClientTraffic(w, r, store, statsClient, singboxStatsClient, inboundID, clientID) {
					applyCoreAsync(ctrl, store)
				}
			} else if len(parts) != 2 || parts[1] != "clients" {
				http.NotFound(w, r)
				return
			} else {
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				created, inbound, ok := createClient(w, r, store, inboundID)
				if ok {
					if db.InboundCore(inbound) == db.CoreSingbox {
						applyXrayAsync(ctrl)
						writeCreatedSingboxResult(w, cfg, r, store, map[string]interface{}{"client": created})
						return
					}
					applyCoreAsync(ctrl, store)
				}
			}
		case http.MethodPatch:
			if len(parts) == 2 && parts[1] == "enabled" {
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				if patchInboundEnabled(w, r, store, inboundID) {
					applyCoreAsync(ctrl, store)
				}
			} else if len(parts) == 4 && parts[1] == "clients" && parts[3] == "enabled" {
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				if patchClientEnabled(w, r, store, inboundID, clientID) {
					applyCoreAsync(ctrl, store)
				}
			} else {
				http.NotFound(w, r)
			}
		case http.MethodPut:
			if len(parts) == 1 {
				// PUT /api/inbounds/{id}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				if updateInbound(w, r, store, inboundID) {
					applyCoreAsync(ctrl, store)
				}
			} else if len(parts) == 3 && parts[1] == "clients" {
				// PUT /api/inbounds/{id}/clients/{clientId}
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				if updateClient(w, r, store, clientID) {
					applyCoreAsync(ctrl, store)
				}
			} else {
				http.NotFound(w, r)
			}
		case http.MethodDelete:
			if len(parts) == 1 {
				// DELETE /api/inbounds/{id}
				inboundID, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil || inboundID <= 0 {
					http.NotFound(w, r)
					return
				}
				if store == nil {
					writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
					return
				}
				if err := store.DeleteInbound(r.Context(), inboundID); err != nil {
					writeJSONError(w, http.StatusNotFound, "inbound_not_found")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
				applyCoreAsync(ctrl, store)
			} else if len(parts) == 3 && parts[1] == "clients" {
				// DELETE /api/inbounds/{id}/clients/{clientId}
				clientID, err := strconv.ParseInt(parts[2], 10, 64)
				if err != nil || clientID <= 0 {
					http.NotFound(w, r)
					return
				}
				if store == nil {
					writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
					return
				}
				if err := store.DeleteClient(r.Context(), clientID); err != nil {
					writeJSONError(w, http.StatusNotFound, "client_not_found")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
				applyCoreAsync(ctrl, store)
			} else {
				http.NotFound(w, r)
			}
		default:
			methodNotAllowed(w)
		}
	}
}

func createClient(w http.ResponseWriter, r *http.Request, store Store, inboundID int64) (db.Client, db.Inbound, bool) {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return db.Client{}, db.Inbound{}, false
	}
	inbound, found, err := findInbound(r.Context(), store, inboundID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
		return db.Client{}, db.Inbound{}, false
	}
	if !found {
		writeJSONError(w, http.StatusNotFound, "inbound_not_found")
		return db.Client{}, db.Inbound{}, false
	}
	var payload struct {
		Email        string `json:"email"`
		UUID         string `json:"uuid"`
		CredentialID string `json:"credential_id"`
		Password     string `json:"password"`
		Enabled      *bool  `json:"enabled"`
		TrafficLimit int64  `json:"traffic_limit"`
		ExpiryAt     int64  `json:"expiry_at"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return db.Client{}, db.Inbound{}, false
	}
	created, err := store.CreateClient(r.Context(), db.CreateClientParams{InboundID: inboundID, Email: payload.Email, UUID: payload.UUID, CredentialID: payload.CredentialID, Password: payload.Password, Enabled: payload.Enabled, TrafficLimit: payload.TrafficLimit, ExpiryAt: payload.ExpiryAt})
	if err != nil {
		if strings.Contains(err.Error(), "duplicate client") {
			writeJSONError(w, http.StatusConflict, "duplicate_client", map[string]interface{}{
				"message": "同一入站下客户端邮箱或凭据已存在，请更换后重试",
			})
			return db.Client{}, db.Inbound{}, false
		}
		writeJSONError(w, http.StatusBadRequest, "create_client_failed")
		return db.Client{}, db.Inbound{}, false
	}
	if db.InboundCore(inbound) != db.CoreSingbox {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)
	}
	return created, inbound, true
}

func patchInboundEnabled(w http.ResponseWriter, r *http.Request, store Store, inboundID int64) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return false
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	updated, err := store.SetInboundEnabled(r.Context(), inboundID, payload.Enabled)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "inbound_not_found")
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
	return true
}

func patchClientEnabled(w http.ResponseWriter, r *http.Request, store Store, inboundID int64, clientID int64) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return false
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	updated, err := store.SetClientEnabled(r.Context(), inboundID, clientID, payload.Enabled)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "client_not_found")
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
	return true
}

func inboundExists(ctx context.Context, store Store, inboundID int64) bool {
	exists, err := store.InboundExists(ctx, inboundID)
	if err != nil {
		return false
	}
	return exists
}

func findInbound(ctx context.Context, store Store, inboundID int64) (db.Inbound, bool, error) {
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return db.Inbound{}, false, err
	}
	for _, inbound := range inbounds {
		if inbound.ID == inboundID {
			return inbound, true, nil
		}
	}
	return db.Inbound{}, false, nil
}

func updateInbound(w http.ResponseWriter, r *http.Request, store Store, inboundID int64) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return false
	}
	var payload db.UpdateInboundParams
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	if err := prepareUpdateInboundRealityKeys(r.Context(), store, inboundID, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "prepare_reality_keys_failed")
		return false
	}
	// Port conflict check (excluding current inbound)
	if payload.Port > 0 {
		conflict, ok, err := store.FindInboundByPort(r.Context(), payload.Port, inboundID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "port_check_failed")
			return false
		}
		if ok {
			writeJSONError(w, http.StatusConflict, "port_conflict", map[string]interface{}{
				"message": "端口 " + strconv.FormatInt(int64(conflict.Port), 10) + " 已被入站 " + strconv.FormatInt(conflict.ID, 10) + " 使用",
			})
			return false
		}
	}
	updated, err := store.UpdateInbound(r.Context(), inboundID, payload)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSONError(w, http.StatusNotFound, "inbound_not_found")
			return false
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
	return true
}

func resetClientTraffic(w http.ResponseWriter, r *http.Request, store Store, statsClient xray.StatsClient, singboxStatsClient singbox.StatsClient, inboundID, clientID int64) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return false
	}
	if !clientBelongsToInbound(r.Context(), store, inboundID, clientID) {
		writeJSONError(w, http.StatusNotFound, "client_not_found")
		return false
	}
	baselines := collectTrafficBaselines(r.Context(), store, statsClient, singboxStatsClient)
	updated, err := store.ResetClientTrafficBaseline(r.Context(), clientID, baselines)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "reset_traffic_failed")
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
	return true
}

func clientBelongsToInbound(ctx context.Context, store Store, inboundID, clientID int64) bool {
	if inboundID <= 0 || clientID <= 0 {
		return false
	}
	inbounds, err := store.ListInbounds(ctx)
	if err != nil {
		return false
	}
	for _, inbound := range inbounds {
		if inbound.ID != inboundID {
			continue
		}
		for _, client := range inbound.Clients {
			if client.ID == clientID {
				return true
			}
		}
		return false
	}
	return false
}

func collectTrafficBaselines(ctx context.Context, store Store, statsClient xray.StatsClient, singboxStatsClient singbox.StatsClient) []db.TrafficRawStat {
	baselines := []db.TrafficRawStat{}
	appendStats := func(stats []xray.TrafficStat) {
		for _, stat := range stats {
			baselines = append(baselines, db.TrafficRawStat{
				Engine: stat.Engine, ScopeType: stat.ScopeType, ScopeKey: stat.ScopeKey,
				RawUp: stat.Uplink, RawDown: stat.Downlink, Status: "waiting",
			})
		}
	}
	if statsClient != nil {
		if stats, err := statsClient.QueryTrafficStats(ctx); err == nil {
			appendStats(stats)
		}
	}
	if singboxStatsClient != nil {
		if stats, err := singboxStatsClient.QueryTrafficStats(ctx); err == nil {
			appendStats(stats)
		}
	}
	return baselines
}

func updateClient(w http.ResponseWriter, r *http.Request, store Store, clientID int64) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return false
	}
	var payload db.UpdateClientParams
	if err := decodeJSONBody(r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return false
	}
	updated, err := store.UpdateClient(r.Context(), clientID, payload)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate client") {
			writeJSONError(w, http.StatusConflict, "duplicate_client", map[string]interface{}{
				"message": "同一入站下客户端邮箱已存在，请更换后重试",
			})
			return false
		}
		writeJSONError(w, http.StatusNotFound, "update_client_failed")
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
	return true
}
