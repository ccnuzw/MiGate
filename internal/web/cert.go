package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/imzyb/MiGate/internal/db"
	certsvc "github.com/imzyb/MiGate/internal/service/cert"
)

type certService interface {
	List(ctx context.Context) ([]db.Certificate, error)
	Get(ctx context.Context, id int64) (db.Certificate, error)
	Status(ctx context.Context) certsvc.StatusResponse
	Preflight(ctx context.Context, req certsvc.IssueRequest) (certsvc.PreflightResult, error)
	Issue(ctx context.Context, req certsvc.IssueRequest) (db.Certificate, certsvc.PreflightResult, error)
	Import(ctx context.Context, req certsvc.ImportRequest) (db.Certificate, error)
	Apply(ctx context.Context, req certsvc.ApplyRequest) ([]db.Inbound, []string, error)
	Delete(ctx context.Context, id int64) error
	Operations(ctx context.Context, certificateID int64, limit int) ([]db.CertificateOperation, error)
	RenewDue(ctx context.Context, days int) (certsvc.RenewResult, error)
}

func certStatusHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, http.StatusOK, certServiceFor(cfg).Status(r.Context()))
	}
}

func certIssueHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var req struct {
			Domain             string   `json:"domain"`
			Domains            []string `json:"domains"`
			Email              string   `json:"email"`
			Confirm            bool     `json:"confirm"`
			AllowSystemChanges bool     `json:"allow_system_changes"`
		}
		if err := decodeJSONBody(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if !req.Confirm || !req.AllowSystemChanges {
			writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
			return
		}
		cert, preflight, err := certServiceFor(cfg).Issue(r.Context(), certsvc.IssueRequest{Domain: req.Domain, Domains: req.Domains, Email: req.Email, Method: "http-01"})
		if err != nil {
			writeCertificateIssueError(w, err, preflight)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"status": cert.Status, "certificate": cert, "preflight": preflight, "domain": firstCertDomain(cert), "cert_path": cert.CertPath, "key_path": cert.KeyPath})
	}
}

func certificatesHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			certs, err := certServiceFor(cfg).List(r.Context())
			if err != nil {
				writeServiceError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"certificates": certs})
		case http.MethodPost:
			var req struct {
				Domains            []string `json:"domains"`
				Domain             string   `json:"domain"`
				Email              string   `json:"email"`
				Confirm            bool     `json:"confirm"`
				AllowSystemChanges bool     `json:"allow_system_changes"`
			}
			if err := decodeJSONBody(r, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			if !req.Confirm || !req.AllowSystemChanges {
				writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
				return
			}
			cert, preflight, err := certServiceFor(cfg).Issue(r.Context(), certsvc.IssueRequest{Domain: req.Domain, Domains: req.Domains, Email: req.Email, Method: "http-01"})
			if err != nil {
				writeCertificateIssueError(w, err, preflight)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]interface{}{"certificate": cert, "preflight": preflight})
		default:
			methodNotAllowed(w)
		}
	}
}

func certificateChildrenHandler(cfg *routerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/certificates/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 1 {
			switch parts[0] {
			case "preflight":
				if r.Method != http.MethodPost {
					methodNotAllowed(w)
					return
				}
				certificatePreflight(w, r, cfg)
				return
			case "import":
				if r.Method != http.MethodPost {
					methodNotAllowed(w)
					return
				}
				certificateImport(w, r, cfg)
				return
			case "renew-due":
				if r.Method != http.MethodPost {
					methodNotAllowed(w)
					return
				}
				certificateRenewDue(w, r, cfg)
				return
			case "inbounds":
				if r.Method != http.MethodGet {
					methodNotAllowed(w)
					return
				}
				certificateInboundTargets(w, r, cfg)
				return
			}
		}
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}
		switch {
		case len(parts) == 1 && r.Method == http.MethodGet:
			cert, err := certServiceFor(cfg).Get(r.Context(), id)
			if err != nil {
				writeServiceError(w, certErrorStatus(err), err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"certificate": cert})
		case len(parts) == 2 && parts[1] == "delete" && r.Method == http.MethodPost:
			var req struct {
				Confirm            bool `json:"confirm"`
				AllowSystemChanges bool `json:"allow_system_changes"`
			}
			if err := decodeJSONBody(r, &req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid_json")
				return
			}
			if !req.Confirm || !req.AllowSystemChanges {
				writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
				return
			}
			if err := certServiceFor(cfg).Delete(r.Context(), id); err != nil {
				writeServiceError(w, certErrorStatus(err), err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		case len(parts) == 2 && parts[1] == "operations" && r.Method == http.MethodGet:
			ops, err := certServiceFor(cfg).Operations(r.Context(), id, parseLimit(r.URL.Query().Get("limit")))
			if err != nil {
				writeServiceError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"operations": ops})
		case len(parts) == 2 && parts[1] == "apply" && r.Method == http.MethodPost:
			certificateApply(w, r, cfg, id)
		case (len(parts) == 1 || (len(parts) == 2 && (parts[1] == "operations" || parts[1] == "apply" || parts[1] == "delete"))) && r.Method != "":
			methodNotAllowed(w)
		default:
			http.NotFound(w, r)
		}
	}
}

func certificatePreflight(w http.ResponseWriter, r *http.Request, cfg *routerConfig) {
	var req struct {
		Domains []string `json:"domains"`
		Domain  string   `json:"domain"`
		Email   string   `json:"email"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	result, err := certServiceFor(cfg).Preflight(r.Context(), certsvc.IssueRequest{Domain: req.Domain, Domains: req.Domains, Email: req.Email})
	if err != nil {
		writeCertificateIssueError(w, err, result)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"preflight": result})
}

func certificateImport(w http.ResponseWriter, r *http.Request, cfg *routerConfig) {
	var req struct {
		Name               string `json:"name"`
		Fullchain          string `json:"fullchain"`
		PrivateKey         string `json:"private_key"`
		Confirm            bool   `json:"confirm"`
		AllowSystemChanges bool   `json:"allow_system_changes"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if !req.Confirm || !req.AllowSystemChanges {
		writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
		return
	}
	cert, err := certServiceFor(cfg).Import(r.Context(), certsvc.ImportRequest{Name: req.Name, Fullchain: req.Fullchain, Key: req.PrivateKey})
	if err != nil {
		writeServiceError(w, certErrorStatus(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"certificate": cert})
}

func certificateApply(w http.ResponseWriter, r *http.Request, cfg *routerConfig, id int64) {
	var req struct {
		InboundIDs         []int64 `json:"inbound_ids"`
		Confirm            bool    `json:"confirm"`
		AllowSystemChanges bool    `json:"allow_system_changes"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if !req.Confirm || !req.AllowSystemChanges {
		writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
		return
	}
	updated, warnings, err := certServiceFor(cfg).Apply(r.Context(), certsvc.ApplyRequest{CertificateID: id, InboundIDs: req.InboundIDs})
	if err != nil {
		writeServiceError(w, certErrorStatus(err), err)
		return
	}
	includeXray, includeSingbox := coresForInbounds(updated)
	payload := map[string]interface{}{"status": "applied", "inbounds": updated, "warnings": warnings}
	writeCoreWriteResult(w, r, cfg, cfg.store, http.StatusOK, payload, includeXray, includeSingbox)
}

func certificateRenewDue(w http.ResponseWriter, r *http.Request, cfg *routerConfig) {
	var req struct {
		Days               int  `json:"days"`
		Confirm            bool `json:"confirm"`
		AllowSystemChanges bool `json:"allow_system_changes"`
	}
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if !req.Confirm || !req.AllowSystemChanges {
		writeJSONError(w, http.StatusForbidden, "confirmation_required", map[string]interface{}{"commands_executed": []string{}})
		return
	}
	result, err := certServiceFor(cfg).RenewDue(r.Context(), req.Days)
	if err != nil {
		writeServiceError(w, certErrorStatus(err), err)
		return
	}
	payload := map[string]interface{}{"status": "checked", "renewal": result}
	ApplyRenewedCertificateCores(r.Context(), cfg, cfg.store, result.Renewed, payload)
	writeJSON(w, http.StatusOK, payload)
}

func certificateInboundTargets(w http.ResponseWriter, r *http.Request, cfg *routerConfig) {
	if cfg == nil || cfg.store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "store_unavailable")
		return
	}
	inbounds, err := cfg.store.ListInbounds(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_inbounds_failed")
		return
	}
	targets := []db.Inbound{}
	for _, inbound := range inbounds {
		if db.NormalizeInboundSecurity(inbound.Protocol, inbound.Security) == "tls" {
			targets = append(targets, inbound)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"inbounds": targets})
}

func certServiceFor(cfg *routerConfig) certService {
	service := certsvc.Service{}
	if cfg != nil {
		service.CertDir = cfg.certDir
		service.LookupIP = cfg.certLookupIP
		service.ListenTCP = cfg.certListenTCP
		service.Issuer = cfg.certIssuer
		if store, ok := cfg.store.(certsvc.Store); ok {
			service.Store = store
		}
	}
	return service
}

func certErrorStatus(err error) int {
	if serviceErr, ok := err.(certsvc.Error); ok {
		switch serviceErr.Code {
		case "domain_required", "email_required", certsvc.CodeInvalidDomain, certsvc.CodeInvalidEmail, certsvc.CodeInvalidCertificate, certsvc.CodePrivateKeyMismatch, "inbound_ids_required", certsvc.CodeDomainNotResolved, certsvc.CodeCertDirNotWritable:
			return http.StatusBadRequest
		case certsvc.CodePreflightFailed:
			if preflightHasFailedCode(serviceErr.Preflight, certsvc.CodeHTTP01PortUnavailable) {
				return http.StatusConflict
			}
			return http.StatusBadRequest
		case certsvc.CodeHTTP01PortUnavailable:
			return http.StatusConflict
		case certsvc.CodeCertificateNotFound, certsvc.CodeInboundNotFound:
			return http.StatusNotFound
		case "certificate_in_use":
			return http.StatusConflict
		case "store_unavailable":
			return http.StatusServiceUnavailable
		default:
			return http.StatusInternalServerError
		}
	}
	return http.StatusInternalServerError
}

func preflightHasFailedCode(preflight *certsvc.PreflightResult, code string) bool {
	if preflight == nil {
		return false
	}
	for _, check := range preflight.Checks {
		if check.Code == code && check.Status == "failed" {
			return true
		}
	}
	return false
}

func writeCertificateIssueError(w http.ResponseWriter, err error, preflight certsvc.PreflightResult) {
	if serviceErr, ok := err.(certsvc.Error); ok && serviceErr.Preflight == nil && len(preflight.Checks) > 0 {
		serviceErr.Preflight = &preflight
		writeServiceError(w, certErrorStatus(serviceErr), serviceErr)
		return
	}
	writeServiceError(w, certErrorStatus(err), err)
}

func coresForInbounds(inbounds []db.Inbound) (bool, bool) {
	includeXray, includeSingbox := false, false
	for _, inbound := range inbounds {
		switch db.InboundCore(inbound) {
		case db.CoreXray:
			includeXray = true
		case db.CoreSingbox:
			includeSingbox = true
		}
	}
	return includeXray, includeSingbox
}

func coresForCertificates(certs []db.Certificate) (bool, bool) {
	includeXray, includeSingbox := false, false
	for _, cert := range certs {
		for _, inbound := range cert.Usages {
			switch db.InboundCore(inbound) {
			case db.CoreXray:
				includeXray = true
			case db.CoreSingbox:
				includeSingbox = true
			}
		}
	}
	return includeXray, includeSingbox
}

func ApplyRenewedCertificateCores(ctx context.Context, cfg *routerConfig, store Store, renewed []db.Certificate, payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	renewed = reloadCertificatesForCoreApply(ctx, store, renewed)
	includeXray, includeSingbox := coresForCertificates(renewed)
	if err := markCoresPending(ctx, cfg, "certificate_renewed", includeXray, includeSingbox); err != nil {
		payload["pending_apply_error"] = "mark_core_pending_failed"
		payload["pending_apply_detail"] = err.Error()
	}
	attachCorePendingApplyResult(ctx, cfg, payload, includeXray, includeSingbox)
	if includeXray || includeSingbox {
		payload["message"] = "certificate renewed; apply core configuration to make it effective"
	}
	return payload
}

type certificateGetter interface {
	GetCertificate(ctx context.Context, id int64) (db.Certificate, error)
}

func reloadCertificatesForCoreApply(ctx context.Context, store Store, certs []db.Certificate) []db.Certificate {
	getter, ok := store.(certificateGetter)
	if !ok || len(certs) == 0 {
		return certs
	}
	loaded := make([]db.Certificate, 0, len(certs))
	for _, cert := range certs {
		if cert.ID <= 0 {
			loaded = append(loaded, cert)
			continue
		}
		fresh, err := getter.GetCertificate(ctx, cert.ID)
		if err != nil {
			loaded = append(loaded, cert)
			continue
		}
		loaded = append(loaded, fresh)
	}
	return loaded
}

func CertificateCoreApplyFunc(options ...Option) func(context.Context, []db.Certificate) map[string]interface{} {
	cfg := routerConfig{
		xrayController: defaultXrayController{},
		singboxRuntime: defaultSingboxRuntime{},
		singboxApplier: tryApplySingboxWithRuntime,
	}
	for _, option := range options {
		option(&cfg)
	}
	return func(ctx context.Context, renewed []db.Certificate) map[string]interface{} {
		return ApplyRenewedCertificateCores(ctx, &cfg, cfg.store, renewed, map[string]interface{}{})
	}
}

func firstCertDomain(cert db.Certificate) string {
	if len(cert.Domains) == 0 {
		return ""
	}
	return cert.Domains[0]
}

func parseLimit(value string) int {
	limit, _ := strconv.Atoi(value)
	return limit
}
