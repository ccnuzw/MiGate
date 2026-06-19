package architecture_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func join(parts ...string) string { return strings.Join(parts, "") }

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(dir, ".."))
}

func read(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func readIfExists(t *testing.T, parts ...string) (string, bool) {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b), true
}

func goFiles(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || path == filepath.Join(root, "internal", "web", "static", "dist") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk go files: %v", err)
	}
	return files
}

func sourceFiles(t *testing.T, roots ...string) []string {
	t.Helper()
	root := repoRoot(t)
	var files []string
	for _, relRoot := range roots {
		base := filepath.Join(root, relRoot)
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case ".git", "node_modules", "dist":
					return fs.SkipDir
				}
				if path == filepath.Join(root, "internal", "web", "static", "dist") {
					return fs.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", relRoot, err)
		}
	}
	return files
}

func TestServiceRunsSinglePrebuiltBinary(t *testing.T) {
	service := read(t, "packaging", "migate.service")
	if !strings.Contains(service, "ExecStart=/usr/local/bin/migate") {
		t.Fatalf("service must run single prebuilt binary:\n%s", service)
	}
	forbidden := []string{"python", "uv", "pip", "npm", "migate-proxy", join("open", "vpn"), "tun", "egress", "remote", "leak", "rollout"}
	lower := strings.ToLower(service)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("service must not contain %q:\n%s", word, service)
		}
	}
}

func TestInstallerDownloadsReleaseTarballOnly(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"migate-linux-${ARCH}.tar.gz",
		"/var/lib/migate",
		"/var/lib/migate/backups",
		"/var/lib/migate/versions.json",
		"/etc/migate/cores",
		"systemctl enable migate",
		"systemctl restart migate",
		"detect_existing_install()",
		"read_existing_config_defaults()",
		"REGENERATE_CONFIG",
		"--dry-run",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer missing %q:\n%s", want, script)
		}
	}
	forbidden := []string{"git clone", "pip install", "uv ", "python3 -m", "npm install", join("open", "vpn"), "migate-proxy", "rollout", "leak", "egress"}
	lower := strings.ToLower(script)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("installer must not contain %q:\n%s", word, script)
		}
	}
}

func TestRuntimeContractDoesNotAdvertiseLegacyPathsOrServices(t *testing.T) {
	for _, file := range []string{
		filepath.Join("packaging", "install.sh"),
		filepath.Join("packaging", "uninstall.sh"),
		filepath.Join("packaging", "migate.service"),
		filepath.Join("cmd", "migate", "main.go"),
		filepath.Join("internal", "web", "core_handlers.go"),
		filepath.Join("internal", "singbox", "manager.go"),
	} {
		content := read(t, file)
		for _, forbidden := range []string{
			"/usr/local/" + "migate",
			"/etc/sing-box/" + "config.json",
			"/usr/local/etc/" + "xray",
			"migate-" + "singbox",
			"ExecStart=/usr/local/bin/xray run -config",
			"ExecStart=/usr/local/bin/sing-box run -c /etc/sing-box/" + "config.json",
		} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain legacy runtime contract %q", file, forbidden)
			}
		}
	}
}

func TestOpsContractDocumentsStandardPathsServicesAndCommands(t *testing.T) {
	architecture := read(t, "docs", "architecture.md")
	for _, want := range []string{
		"MiGate Ops Contract",
		"/etc/migate/panel.json",
		"/etc/migate/cores/xray.json",
		"/etc/migate/cores/sing-box.json",
		"/var/lib/migate/migate.db",
		"/var/lib/migate/versions.json",
		"/var/lib/migate/backups",
		"/var/log/migate",
		"/run/migate",
		"`migate`",
		"`migate-xray`",
		"`migate-sing-box`",
		"read-only checks",
		"dangerous writes",
		"`mg update --check` delegates to `migate-install --check`",
	} {
		if !strings.Contains(architecture, want) {
			t.Fatalf("Ops Contract missing marker %q", want)
		}
	}
}

func TestCLIOpsContractUsesRuntimePathsAndServices(t *testing.T) {
	main := read(t, "cmd", "migate", "main.go")
	for _, want := range []string{
		"paths.PanelService",
		"paths.XrayService",
		"paths.SingboxService",
		"paths.ConfigDir",
		"paths.Database",
		"paths.VersionsFile",
		"paths.BackupDir",
		"paths.XrayConfig",
		"paths.SingboxConfig",
		"paths.LogDir",
		"paths.RunDir",
		"paths.XrayBinary",
		"paths.SingboxBinary",
		"paths.Installer",
		"paths.Uninstaller",
	} {
		if !strings.Contains(main, want) {
			t.Fatalf("cmd/migate main.go missing Runtime Contract marker %q", want)
		}
	}
	if !strings.Contains(main, "return []string{paths.ConfigDir, paths.Database, paths.VersionsFile}") {
		t.Fatal("CLI backup default scope must remain /etc/migate, /var/lib/migate/migate.db, and /var/lib/migate/versions.json through internal/paths")
	}
	for _, forbidden := range []string{"/etc/sing-box/config.json", "/usr/local/migate", "/usr/local/etc/xray", "migate-singbox"} {
		if strings.Contains(main, forbidden) {
			t.Fatalf("cmd/migate main.go must not contain legacy path/service %q", forbidden)
		}
	}
}

func TestInstallUninstallDocsAndScriptsKeepOpsContract(t *testing.T) {
	files := []string{
		filepath.Join("packaging", "install.sh"),
		filepath.Join("packaging", "uninstall.sh"),
		filepath.Join("README.md"),
		filepath.Join("docs", "install.md"),
		filepath.Join("docs", "architecture.md"),
	}
	for _, file := range files {
		content := read(t, file)
		for _, want := range []string{"/etc/migate", "/var/lib/migate", "migate-xray", "migate-sing-box"} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing standard ops marker %q", file, want)
			}
		}
		for _, forbidden := range []string{"/etc/sing-box/config.json", "/usr/local/migate", "/usr/local/etc/xray", "migate-singbox"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain legacy ops marker %q", file, forbidden)
			}
		}
	}

	uninstaller := read(t, "packaging", "uninstall.sh")
	for _, want := range []string{
		"Default uninstall keeps:",
		`if [ "$PURGE" -eq 1 ]; then`,
		`rm -rf "$MIGATE_CONFIG_DIR"`,
		`rm -rf "$MIGATE_DATA_DIR"`,
		`rm -rf "$MIGATE_LOG_DIR"`,
		`rm -rf "$MIGATE_RUN_DIR"`,
		"Keeping MiGate config/data/logs",
	} {
		if !strings.Contains(uninstaller, want) {
			t.Fatalf("uninstaller purge/keep contract missing %q", want)
		}
	}

	installer := read(t, "packaging", "install.sh")
	for _, want := range []string{
		`INSTALL_LOCK="${MIGATE_INSTALL_LOCK:-/run/migate/install.lock}"`,
		`if [ "$DRY_RUN" -eq 1 ] && [ "$INSTALL_LOCK" = "/run/migate/install.lock" ]; then`,
		`INSTALL_LOCK="${TMPDIR:-/tmp}/migate-install.$$.lock"`,
	} {
		if !strings.Contains(installer, want) {
			t.Fatalf("installer dry-run lock contract missing %q", want)
		}
	}
}

func TestRemovedLegacyRuntimeCodeIsFullyRemoved(t *testing.T) {
	root := repoRoot(t)
	for _, removedDir := range []string{
		filepath.Join(root, "internal", join("vpn", "gate")),
	} {
		if _, err := os.Stat(removedDir); !os.IsNotExist(err) {
			t.Fatalf("removed VPN feature implementation directory must be removed: %s", removedDir)
		}
	}

	if _, exists := readIfExists(t, "internal", "web", "static", "app.js"); exists {
		t.Fatal("removed internal/web/static/app.js must stay absent after frontend split")
	}

	for _, file := range []string{
		filepath.Join("internal", "db", "store.go"),
		filepath.Join("internal", "web", "router.go"),
		filepath.Join("internal", "web", "auth.go"),
		filepath.Join("internal", "xray", "config.go"),
		filepath.Join("cmd", "migate", "main.go"),
		filepath.Join("packaging", "install.sh"),
		filepath.Join("web", "src", "App.tsx"),
	} {
		content := strings.ToLower(read(t, file))
		for _, forbidden := range []string{join("vpn", "gate"), join("vpn", " gate"), join("soft", "ether"), join("micro", "socks"), join("vpn", "cmd"), join("vpn", "client")} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain removed VPN feature marker %q", file, forbidden)
			}
		}
	}
}

func TestRemovedLegacyAPIErrorCompatibilityIsFullyRemoved(t *testing.T) {
	for _, file := range sourceFiles(t, ".") {
		content := read(t, file)
		for _, forbidden := range []string{join("legacy", "_error"), join("Legacy", "Error")} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not contain removed legacy API error marker %q", file, forbidden)
			}
		}
	}
}

func TestFrontendAPICallsStayBehindClientBoundary(t *testing.T) {
	for _, file := range sourceFiles(t, filepath.Join("web", "src")) {
		if strings.Contains(file, string(filepath.Separator)+"api"+string(filepath.Separator)) || strings.Contains(file, ".test.") {
			continue
		}
		content := read(t, file)
		if strings.Contains(content, "fetch(") {
			t.Fatalf("%s must not call fetch directly; route API traffic through web/src/api", file)
		}
	}
}

func TestFrontendAPIEndpointStringsStayInAPIModules(t *testing.T) {
	for _, file := range sourceFiles(t, filepath.Join("web", "src")) {
		if strings.Contains(file, string(filepath.Separator)+"api"+string(filepath.Separator)) || strings.Contains(file, ".test.") {
			continue
		}
		content := read(t, file)
		for _, marker := range []string{`"/api/`, `'/api/`, "`/api/", `"/sub/`, `'/sub/`, "`/sub/"} {
			if strings.Contains(content, marker) {
				t.Fatalf("%s must keep API endpoint strings in web/src/api, found marker %q", file, marker)
			}
		}
	}
}

func TestFrontendComponentsUseQueryInvalidationHelpers(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("web", "src", "lib", "queryInvalidation.ts"): true,
	}
	for _, file := range sourceFiles(t, filepath.Join("web", "src")) {
		if strings.Contains(file, ".test.") || allowed[file] {
			continue
		}
		content := read(t, file)
		if strings.Contains(content, "queryClient.invalidateQueries") {
			t.Fatalf("%s must centralize query invalidation in web/src/lib/queryInvalidation.ts", file)
		}
	}
}

func TestReadmeIncludesSimpleInstallAndUsage(t *testing.T) {
	readme := read(t, "README.md")
	for _, want := range []string{
		"bash <(curl -Ls https://raw.githubusercontent.com/imzyb/MiGate/main/packaging/install.sh)",
		"MIGATE_VERSION=",
		"http://127.0.0.1:9999/panel",
		"reverse proxy",
		"public_host",
		"Web path, default `/panel`",
		"systemctl status migate",
		"systemctl restart migate",
		"/etc/migate/panel.json",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README missing simple usage marker %q", want)
		}
	}
	for _, forbiddenName := range []string{join("MiGate Go", " Lite"), "Go Lite"} {
		if strings.Contains(readme, forbiddenName) {
			t.Fatalf("README should use MiGate as the product name, found %q", forbiddenName)
		}
	}
}

func TestArchitectureDocumentsExist(t *testing.T) {
	docs := map[string][]string{
		filepath.Join("docs", "architecture.md"):    {"internal/config", "strict schema", "service layer", "Route table"},
		filepath.Join("docs", "api-contract.md"):    {"Error Responses", "Dangerous Operations", "route table"},
		filepath.Join("docs", "config-contract.md"): {"strict schema", "panel_port", "database_path"},
	}
	for doc, markers := range docs {
		content := read(t, doc)
		for _, marker := range markers {
			if !strings.Contains(content, marker) {
				t.Fatalf("%s missing marker %q", doc, marker)
			}
		}
	}
}

func TestRouteTableIsAPIContractSource(t *testing.T) {
	routes := read(t, "internal", "web", "routes_contract.go")
	router := read(t, "internal", "web", "router.go")
	for _, want := range []string{"type Route struct", "func routeTable()", "func registerAPIRoutes"} {
		if !strings.Contains(routes, want) {
			t.Fatalf("routes_contract.go missing route table marker %q", want)
		}
	}
	if !strings.Contains(router, "registerAPIRoutes(mux, &cfg, trafficCache, coreCache)") {
		t.Fatal("router.go must register API routes through route table")
	}
	if strings.Contains(router, `mux.HandleFunc("/api/`) {
		t.Fatal("router.go must not hand-register API routes outside route table")
	}
}

func TestCmdDoesNotOwnPanelConfigPersistence(t *testing.T) {
	main := read(t, "cmd", "migate", "main.go")
	for _, forbidden := range []string{"type panelConfig struct", "func readPanelConfig", "func writePanelConfig", "panelconfig.WriteFile", "os.WriteFile("} {
		if strings.Contains(main, forbidden) {
			t.Fatalf("cmd/migate must not own panel config persistence, found %q", forbidden)
		}
	}
	if !strings.Contains(main, `"github.com/imzyb/MiGate/internal/config"`) {
		t.Fatal("cmd/migate must depend on internal/config for panel config persistence")
	}
}

func TestDirectExecCommandIsControlled(t *testing.T) {
	allowed := map[string]string{
		filepath.Join("internal", "runtime", "command", "command.go"): "central runtime command adapter owns exec.CommandContext and output truncation",
		filepath.Join("internal", "runtime", "script", "script.go"):   "runtime script adapter owns bash stdin and systemd-run --pipe execution",
	}
	for _, file := range goFiles(t) {
		content := read(t, file)
		if strings.Contains(content, "exec.Command") || strings.Contains(content, "exec.CommandContext") || strings.Contains(content, "exec.LookPath") {
			if allowed[file] == "" {
				t.Fatalf("%s must use internal/runtime/command instead of direct exec.Command", file)
			}
		}
	}
}

func TestServiceLayerDoesNotDependOnHTTPHandlers(t *testing.T) {
	for _, file := range goFiles(t) {
		if !strings.HasPrefix(file, filepath.Join("internal", "service")+string(filepath.Separator)) {
			continue
		}
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		content := read(t, file)
		for _, forbidden := range []string{"http.ResponseWriter", "http.Handler", "http.HandlerFunc", "net/http/httptest"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not depend on HTTP handler types, found %q", file, forbidden)
			}
		}
		if strings.Contains(content, "*http.Request") && !strings.HasPrefix(file, filepath.Join("internal", "service", "update")+string(filepath.Separator)) {
			t.Fatalf("%s must not depend on inbound HTTP request types; only internal/service/update may use *http.Request for outbound release checks", file)
		}
		if strings.Contains(content, `"net/http"`) && !strings.HasPrefix(file, filepath.Join("internal", "service", "update")+string(filepath.Separator)) {
			t.Fatalf("%s must not import net/http; only internal/service/update may use it as an outbound release-check client", file)
		}
	}
}

func TestPanelConfigWritesGoThroughConfigPackage(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("internal", "config", "config.go"):      true,
		filepath.Join("internal", "panelconfig", "writer.go"): true,
	}
	for _, file := range goFiles(t) {
		content := read(t, file)
		if referencesPanelConfig(content) && usesFileWriteAPI(content) {
			if !allowed[file] {
				t.Fatalf("%s must write panel config through internal/config or internal/panelconfig", file)
			}
		}
	}
}

func TestSettingsConfigDoesNotUseArbitraryMapPersistence(t *testing.T) {
	for _, file := range []string{
		filepath.Join("internal", "web", "settings.go"),
		filepath.Join("internal", "web", "cert.go"),
		filepath.Join("internal", "config", "config.go"),
	} {
		content := read(t, file)
		for _, forbidden := range []string{"UpdateRaw", "map[string]interface{}) error"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s must not use arbitrary map persistence marker %q", file, forbidden)
			}
		}
	}
}

func referencesPanelConfig(content string) bool {
	for _, marker := range []string{
		"paths.PanelConfig",
		"PanelConfig",
		"panel.json",
		"/etc/migate/panel.json",
	} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func usesFileWriteAPI(content string) bool {
	for _, marker := range []string{
		"panelconfig.WriteFile",
		"os.WriteFile(",
		"os.OpenFile(",
		"os.Create(",
	} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func TestWebJSONResponsesUseHelperOrAllowlist(t *testing.T) {
	allowed := map[string]bool{
		filepath.Join("internal", "web", "response.go"): true,
	}
	for _, file := range goFiles(t) {
		if !strings.HasPrefix(file, filepath.Join("internal", "web")+string(filepath.Separator)) {
			continue
		}
		content := read(t, file)
		if strings.Contains(content, "json.NewEncoder(w).Encode") && !allowed[file] {
			t.Fatalf("%s must use internal/web response helpers for JSON responses", file)
		}
	}
}

func TestDatabaseStoreIsSplitByResponsibility(t *testing.T) {
	store := read(t, "internal", "db", "store.go")
	if strings.Contains(store, "CREATE TABLE") || strings.Contains(store, "CREATE INDEX") {
		t.Fatal("internal/db/store.go must stay an entrypoint, not own schema DDL")
	}
	if strings.Contains(store, "traffic_samples") || strings.Contains(store, "traffic_states") ||
		strings.Contains(store, "routing_rules") || strings.Contains(store, "token_blacklist") {
		t.Fatal("internal/db/store.go must not absorb traffic, routing, or session table logic")
	}
	for _, forbidden := range []string{
		"func (s *Store) CreateInbound",
		"func (s *Store) UpdateInbound",
		"func (s *Store) ListInbounds",
		"func (s *Store) CreateClient",
		"func (s *Store) UpdateClient",
		"func (s *Store) GetSubscriptionByToken",
		"func (s *Store) ListOutbounds",
		"func (s *Store) CreateOutbound",
	} {
		if strings.Contains(store, forbidden) {
			t.Fatalf("internal/db/store.go must not absorb repository method %q", forbidden)
		}
	}
	for _, want := range []string{"type Store struct", "func Open(", "func (s *Store) Close() error"} {
		if !strings.Contains(store, want) {
			t.Fatalf("internal/db/store.go missing store entrypoint marker %q", want)
		}
	}

	schema := read(t, "internal", "db", "schema.go")
	for _, want := range []string{
		"func (s *Store) migrate(ctx context.Context) error",
		"CREATE TABLE IF NOT EXISTS inbounds",
		"CREATE TABLE IF NOT EXISTS clients",
		"CREATE TABLE IF NOT EXISTS outbounds",
		"CREATE TABLE IF NOT EXISTS routing_rules",
		"CREATE TABLE IF NOT EXISTS token_blacklist",
		"CREATE TABLE IF NOT EXISTS traffic_states",
		"CREATE TABLE IF NOT EXISTS traffic_samples",
		"CREATE INDEX IF NOT EXISTS idx_traffic_samples_lookup",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("internal/db/schema.go must own schema marker %q", want)
		}
	}

	for _, file := range goFiles(t) {
		if !strings.HasPrefix(file, filepath.Join("internal", "db")+string(filepath.Separator)) {
			continue
		}
		content := read(t, file)
		if (strings.Contains(content, "CREATE TABLE IF NOT EXISTS") || strings.Contains(content, "CREATE INDEX IF NOT EXISTS")) &&
			file != filepath.Join("internal", "db", "schema.go") {
			t.Fatalf("%s must not own db schema DDL; keep schema initialization in internal/db/schema.go", file)
		}
		if strings.Contains(content, "traffic_samples") || strings.Contains(content, "traffic_states") {
			if file != filepath.Join("internal", "db", "schema.go") && file != filepath.Join("internal", "db", "traffic.go") {
				t.Fatalf("%s must not own traffic_samples/traffic_states logic", file)
			}
		}
		if strings.Contains(content, "token_blacklist") {
			if file != filepath.Join("internal", "db", "schema.go") && file != filepath.Join("internal", "db", "sessions.go") {
				t.Fatalf("%s must not own token_blacklist/session logic", file)
			}
		}
	}
}

func TestDatabaseDomainLogicLivesInNamedFiles(t *testing.T) {
	traffic := read(t, "internal", "db", "traffic.go")
	for _, want := range []string{
		"func (s *Store) ApplyTrafficRawStats",
		"func (s *Store) ResetClientTrafficBaseline",
		"func (s *Store) ListTrafficSamples",
		"func (s *Store) ListTrafficStates",
	} {
		if !strings.Contains(traffic, want) {
			t.Fatalf("internal/db/traffic.go missing traffic marker %q", want)
		}
	}

	routing := read(t, "internal", "db", "routing.go")
	for _, want := range []string{
		"func (s *Store) CreateRoutingRule",
		"func (s *Store) UpdateRoutingRule",
		"func (s *Store) DeleteRoutingRule",
		"func (s *Store) resolveRoutingRuleSource",
		"func (s *Store) resolveRoutingOutbound",
	} {
		if !strings.Contains(routing, want) {
			t.Fatalf("internal/db/routing.go missing routing marker %q", want)
		}
	}

	sessions := read(t, "internal", "db", "sessions.go")
	for _, want := range []string{
		"type BlacklistedSession struct",
		"func (s *Store) AddToBlacklist",
		"func (s *Store) IsBlacklisted",
		"func (s *Store) ListActiveSessions",
		"func (s *Store) RevokeSession",
	} {
		if !strings.Contains(sessions, want) {
			t.Fatalf("internal/db/sessions.go missing session marker %q", want)
		}
	}

	inbounds := read(t, "internal", "db", "inbounds.go")
	for _, want := range []string{
		"func (s *Store) CreateInbound",
		"func (s *Store) UpdateInbound",
		"func (s *Store) DeleteInbound",
		"func (s *Store) ListInbounds",
		"func (s *Store) FindInboundByPort",
	} {
		if !strings.Contains(inbounds, want) {
			t.Fatalf("internal/db/inbounds.go missing inbound marker %q", want)
		}
	}

	clients := read(t, "internal", "db", "clients.go")
	for _, want := range []string{
		"func (s *Store) CreateClient",
		"func (s *Store) UpdateClient",
		"func (s *Store) DeleteClient",
		"func (s *Store) SetClientEnabled",
		"func (s *Store) GetSubscriptionByClientUUID",
		"func (s *Store) GetSubscriptionByToken",
	} {
		if !strings.Contains(clients, want) {
			t.Fatalf("internal/db/clients.go missing client marker %q", want)
		}
	}

	outbounds := read(t, "internal", "db", "outbounds.go")
	for _, want := range []string{
		"func (s *Store) ListOutbounds",
		"func (s *Store) CreateOutbound",
		"func (s *Store) UpdateOutbound",
		"func (s *Store) DeleteOutbound",
		"func (s *Store) ReorderOutbounds",
		"func (s *Store) SetOutboundEnabled",
	} {
		if !strings.Contains(outbounds, want) {
			t.Fatalf("internal/db/outbounds.go missing outbound marker %q", want)
		}
	}
}

func TestCoreInstallPlansLiveInCoreAdmin(t *testing.T) {
	webHandler := read(t, "internal", "web", "core_handlers.go")
	coreadminSource := read(t, "internal", "service", "coreadmin", "install.go")
	for _, marker := range []string{
		"download Xray release",
		"install_migate_default_xray_config",
		"install_migate_default_singbox_config",
		"Xray-linux-${asset_arch}.zip",
		"sing-box-${version}-linux",
	} {
		if strings.Contains(webHandler, marker) {
			t.Fatalf("internal/web/core_handlers.go must not own core install script marker %q", marker)
		}
		if !strings.Contains(coreadminSource, marker) {
			t.Fatalf("internal/service/coreadmin must own core install script marker %q", marker)
		}
	}
}
