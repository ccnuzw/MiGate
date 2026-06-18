package packaging_test

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"os/exec"
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

func TestInstallerIsProductizedReleaseInstaller(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"log_info()",
		"section()",
		"detect_os()",
		"detect_arch()",
		"detect_systemd()",
		"dependency_status()",
		"detect_existing_install()",
		"interactive_menu()",
		"升级 MiGate，并保留现有配置",
		"重装 MiGate，并重新生成面板配置",
		"只修复 migate systemd 服务",
		"只安装/修复 Xray",
		"只安装/修复 sing-box",
		"/etc/migate/panel.json",
		"panel_port",
		"panel_username",
		"panel_password",
		"web_base_path",
		"migate-linux-${ARCH}.tar.gz",
		"systemctl enable migate",
		"systemctl restart migate",
		"MIGATE_PANEL_BIND_HOST=0.0.0.0",
		"mktemp /usr/local/bin/.migate-uninstall.XXXXXX",
		"mv -f \"$uninstaller_tmp\" \"$UNINSTALLER_BIN\"",
		"ln -sf \"$MIGATE_BIN\" \"$MIGATE_LINK\"",
		"CLI 命令",
		"WebUI",
		"xray.json",
		"/usr/local/etc/xray/xray.json",
		"ln -sf \"${INSTALL_DIR}/xray.json\" /usr/local/etc/xray/xray.json",
		"install_xray",
		"Xray-linux-${xray_asset_arch}.zip",
		"hash-password",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer missing %q", want)
		}
	}

	forbidden := []string{"git clone", "pip install", "uv ", "python3 -m", "npm install", "go build", "Xray-install/raw/main/install-release.sh", join("open", "vpn"), "migate-proxy", "rollout", "leak", "egress", "armv7"}
	lower := strings.ToLower(script)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("installer must not contain %q", word)
		}
	}
	for _, forbiddenName := range []string{join("MiGate Go", " Lite"), "Go Lite"} {
		if strings.Contains(script, forbiddenName) {
			t.Fatalf("installer should use MiGate as the product name, found %q", forbiddenName)
		}
	}
}

func TestInstallerSupportsNonInteractiveActionsAndDryRun(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"--yes, -y",
		"--install",
		"--upgrade, --update",
		"--uninstall",
		"--repair-service",
		"--install-xray",
		"--install-singbox",
		"--dry-run",
		"DRY_RUN=0",
		"run_cmd()",
		"[DRY-RUN]",
		"install_release_flow",
		"repair_service_flow",
		"uninstall_flow",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer non-interactive/dry-run contract missing %q", want)
		}
	}
}

func TestInstallerPreservesExistingConfigByDefault(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"read_existing_config_defaults()",
		"if [ -f \"$CONFIG_PATH\" ] && [ \"$REGENERATE_CONFIG\" -ne 1 ]; then",
		"保留已有配置",
		"使用已有配置，不重新生成 panel.json",
		"--fresh-config",
		"REGENERATE_CONFIG=1",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer config preservation contract missing %q", want)
		}
	}
}

func TestInstallerFinishMessageNormalizesWebUIPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{name: "missing leading slash", path: "migate", want: ":9999/migate"},
		{name: "leading slash", path: "/migate", want: ":9999/migate"},
		{name: "trailing slash", path: "/migate/", want: ":9999/migate"},
		{name: "root", path: "/", want: ":9999/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := repoRoot(t)
			tmp := t.TempDir()
			configPath := filepath.Join(tmp, "panel.json")
			if err := os.WriteFile(configPath, []byte(`{"panel_port":9999,"panel_username":"admin","web_base_path":"`+tc.path+`"}`), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			cmd := exec.Command("bash", filepath.Join(root, "packaging", "install.sh"), "--upgrade", "--yes", "--dry-run")
			cmd.Dir = root
			cmd.Env = append(os.Environ(),
				"MIGATE_CONFIG_PATH="+configPath,
				"MIGATE_CONFIG_DIR="+tmp,
				"MIGATE_INSTALL_DIR="+tmp,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dry-run upgrade failed: %v\n%s", err, output)
			}
			out := string(output)
			if !strings.Contains(out, "WebUI 地址:") || !strings.Contains(out, tc.want) {
				t.Fatalf("finish message missing normalized WebUI path %q:\n%s", tc.want, out)
			}
			proxyTarget := "反向代理到 0.0.0.0:9999" + strings.TrimPrefix(tc.want, ":9999")
			if !strings.Contains(out, proxyTarget) {
				t.Fatalf("finish message missing normalized reverse proxy target %q:\n%s", proxyTarget, out)
			}
			if strings.Contains(out, ":9999migate") || strings.Contains(out, ":9999//migate") {
				t.Fatalf("finish message contains malformed URL path:\n%s", out)
			}
		})
	}
}

func TestInstallerConfigPathsFollowInstallDir(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"\"database_path\": \"$(json_escape \"$INSTALL_DIR\")/migate.db\"",
		"\"xray_config_path\": \"$(json_escape \"$INSTALL_DIR\")\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer config path contract missing %q", want)
		}
	}
}

func TestInstallerDetectsCorePathsVersionsAndServices(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"detect_core()",
		"core_binary_path()",
		"\"/usr/local/bin/${command_name}\" \"/usr/bin/${command_name}\"",
		"core_version()",
		"systemctl list-unit-files \"${service_name}.service\"",
		"if detect_core \"Xray\" \"xray\" \"xray\"; then XRAY_FOUND=1; else XRAY_FOUND=0; fi",
		"if detect_core \"sing-box\" \"sing-box\" \"sing-box\"; then SINGBOX_FOUND=1; else SINGBOX_FOUND=0; fi",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer core detection contract missing %q", want)
		}
	}
}

func TestInstallerPromptsForMissingCoresByDefaultAndPreservesExistingCores(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"XRAY_FOUND=0",
		"SINGBOX_FOUND=0",
		"CORE_PROMPTS_CONFIRMED=0",
		"confirm_yes \"未检测到 Xray，是否安装 Xray？\"",
		"confirm_yes \"未检测到 sing-box，是否安装 sing-box？\"",
		"confirm_no \"检测到 Xray 已安装，是否重新安装/修复 Xray？\"",
		"confirm_no \"检测到 sing-box 已安装，是否重新安装/修复 sing-box？\"",
		"保留现有 Xray 安装。",
		"保留现有 sing-box 安装。",
		"CORE_PROMPTS_CONFIRMED=1",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer core prompt contract missing %q", want)
		}
	}
}

func TestInstallerGeneratesRandomPasswordWhenBlank(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"generate_password()",
		"PANEL_PASSWORD=\"$(generate_password)\"",
		"未输入密码，已生成随机密码。",
		"管理员密码",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer random password contract missing %q", want)
		}
	}
	for _, forbidden := range []string{"super-secret-password", "hidden default"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("installer must not contain fixed/default password marker %q", forbidden)
		}
	}
}

func TestInstallerUsesPanelBasePath(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"prompt_line \"Web base path [${WEB_BASE_PATH:-/panel}]: \"",
		"WEB_BASE_PATH=\"${input_web_base_path:-${WEB_BASE_PATH:-/panel}}\"",
		"normalize_web_base_path",
		"WEB_BASE_PATH=\"$(normalize_web_base_path \"$WEB_BASE_PATH\")\"",
		"WebUI 地址",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer /panel web base path contract missing %q", want)
		}
	}
}

func TestInstallerOffersSingBoxRuntime(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"install_singbox",
		"confirm_yes \"未检测到 sing-box，是否安装 sing-box？\"",
		"sing-box.service",
		"LEGACY_SINGBOX_SERVICE_PATH",
		"LEGACY_SINGBOX_SERVICE_DROPIN_DIR",
		"ExecStart=/usr/local/bin/sing-box run -c /etc/sing-box/config.json",
		"systemctl stop migate-singbox",
		"systemctl disable migate-singbox",
		"rm -f \"$LEGACY_SINGBOX_SERVICE_PATH\"",
		"rm -rf \"$LEGACY_SINGBOX_SERVICE_DROPIN_DIR\"",
		"systemctl reset-failed migate-singbox",
		"systemctl enable sing-box",
		"/usr/local/bin/sing-box check -c /etc/sing-box/config.json",
		"sing-box 配置校验失败，已跳过服务启动。",
		"journalctl -u sing-box -n 80 --no-pager",
		"sing-box 安装/修复完成",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer sing-box runtime contract missing %q", want)
		}
	}
	serviceWrite := strings.Index(script, "cat > \"$SINGBOX_SERVICE_PATH\"")
	if serviceWrite < 0 || !strings.Contains(script[serviceWrite:], "systemctl daemon-reload") {
		t.Fatalf("installer must daemon-reload after writing sing-box service")
	}
	if strings.Index(script, "/usr/local/bin/sing-box check -c /etc/sing-box/config.json") > strings.Index(script, "systemctl start sing-box") {
		t.Fatalf("installer must check sing-box config before starting service")
	}
	checkBlock := script[strings.Index(script, "if ! /usr/local/bin/sing-box check -c /etc/sing-box/config.json; then"):]
	if !strings.Contains(checkBlock, "已跳过服务启动") || strings.Index(checkBlock, "else") > strings.Index(checkBlock, "systemctl start sing-box") {
		t.Fatalf("installer must skip sing-box service start when config check fails")
	}
}

func TestInstallerConfiguresBoundedLogRetention(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	service := read(t, "packaging", "migate.service")
	for _, want := range []string{
		"configure_log_retention()",
		"SystemMaxUse=128M",
		"RuntimeMaxUse=64M",
		"MaxRetentionSec=14day",
		"journalctl --vacuum-size=128M",
		"/var/log/migate-update.log",
		"size 5M",
		"rotate 3",
		"copytruncate",
		"configure_log_retention",
		"StandardOutput=journal",
		"StandardError=journal",
		"LogRateLimitIntervalSec=30s",
		"LogRateLimitBurst=200",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer log retention contract missing %q", want)
		}
	}
	for _, want := range []string{
		"StandardOutput=journal",
		"StandardError=journal",
		"LogRateLimitIntervalSec=30s",
		"LogRateLimitBurst=200",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("packaged systemd unit log limit missing %q", want)
		}
	}
	if strings.Index(script, "configure_log_retention") > strings.Index(script, "write_systemd_service") {
		t.Fatalf("installer should define log retention before service-writing flow")
	}
}

func TestInstallerCompletionPrintsSaveableInstallSummary(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"安装完成，请保存以下信息",
		"安装目录",
		"面板监听",
		"Web base path",
		"WebUI 地址",
		"管理员用户",
		"管理员密码",
		"面板配置",
		"数据库",
		"Xray 配置",
		"Xray 二进制",
		"sing-box 配置",
		"sing-box 二进制",
		"安装器",
		"卸载器",
		"MiGate 服务文件",
		"常用命令",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer completion summary missing %q", want)
		}
	}
}

func TestInstallerDoesNotOfferArchivedRuntimeDependencies(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, forbidden := range []string{
		"install_" + join("vpn", "gate") + "_runtime_dependencies",
		"是否安装 removed VPN feature runtime 依赖？[Y/n]",
		join("micro", "socks"),
		join("soft", "ether", "-vpn", "client"),
		join("soft", "ether", "-vpn", "cmd"),
		"dhclient",
		join("vpn", "cmd"),
		join("vpn", "client"),
		"removed VPN feature runtime dependencies:",
	} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("installer must not offer removed VPN feature runtime dependency %q", forbidden)
		}
	}
}

func TestInstallerDownloadsReleaseAssetAndVerifiesChecksum(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"MIGATE_VERSION:-latest",
		"release_base_url()",
		"latest_release_tag()",
		"ensure_latest_release_version",
		"releases/latest/download",
		"releases/download/%s",
		"CHECKSUM_URL",
		"checksums.txt",
		"download_file \"$CHECKSUM_URL\"",
		"grep \"migate-linux-${ARCH}.tar.gz\"",
		"verify_sha256 \"${ARTIFACT}.sha256\" \"$TMP\"",
		"tar --no-same-owner -xzf \"$TMP/migate-linux-${ARCH}.tar.gz\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer release checksum contract missing %q", want)
		}
	}
	if strings.Index(script, "verify_sha256 \"${ARTIFACT}.sha256\" \"$TMP\"") > strings.Index(script, "tar --no-same-owner -xzf \"$TMP/migate-linux-${ARCH}.tar.gz\"") {
		t.Fatalf("installer must verify checksum before extracting MiGate release archive")
	}
}

func TestInstallerVerifiesSingBoxArchiveChecksumBeforeExtracting(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"sb_artifact=\"sing-box-${sb_version}-linux-${sb_asset_arch}.tar.gz\"",
		"sb_release_api_url=\"https://api.github.com/repos/SagerNet/sing-box/releases/tags/v${sb_version}\"",
		"curl -fL \"$sb_url\" -o \"$tmp_sb/$sb_artifact\"",
		"curl -fsSL \"$sb_release_api_url\" -o \"$tmp_sb/release.json\"",
		"/\"name\": \"/ { in_asset=0 }",
		"printf '%s  %s\\n' \"$sb_digest\" \"$sb_artifact\" > \"$tmp_sb/$sb_artifact.sha256\"",
		"verify_sha256 \"$sb_artifact.sha256\" \"$tmp_sb\"",
		"tar --no-same-owner -xzf \"$tmp_sb/$sb_artifact\" -C \"$tmp_sb\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer sing-box checksum contract missing %q", want)
		}
	}
	if strings.Index(script, "verify_sha256 \"$sb_artifact.sha256\" \"$tmp_sb\"") > strings.Index(script, "tar --no-same-owner -xzf \"$tmp_sb/$sb_artifact\"") {
		t.Fatalf("installer must verify sing-box checksum before extracting archive")
	}
}

func TestInstallerDoesNotAbortMainFlowWhenOptionalCoreInstallFails(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"maybe_install_core()",
		"( set -e; \"$installer\" )",
		"${label} 安装/修复失败",
		"MiGate 安装/升级将继续",
		"if [ \"$EXPLICIT_INSTALL_XRAY\" -eq 1 ]; then install_xray; else maybe_install_core \"Xray\" install_xray; fi",
		"if [ \"$EXPLICIT_INSTALL_SINGBOX\" -eq 1 ]; then install_singbox; else maybe_install_core \"sing-box\" install_singbox; fi",
		"install-singbox-only)",
		"install_singbox",
		"install-xray-only)",
		"install_xray",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer optional core failure contract missing %q", want)
		}
	}
	if strings.Index(script, "if [ \"$EXPLICIT_INSTALL_XRAY\" -eq 1 ]; then install_xray; else maybe_install_core \"Xray\" install_xray; fi") > strings.Index(script, "section \"服务启动\"") {
		t.Fatalf("installer main flow must handle optional Xray failure before service startup")
	}
	if strings.Index(script, "if [ \"$EXPLICIT_INSTALL_SINGBOX\" -eq 1 ]; then install_singbox; else maybe_install_core \"sing-box\" install_singbox; fi") > strings.Index(script, "section \"服务启动\"") {
		t.Fatalf("installer main flow must handle optional sing-box failure before service startup")
	}
}

func TestInstallerExplicitCoreInstallFlagsRemainStrict(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"EXPLICIT_INSTALL_XRAY=0",
		"EXPLICIT_INSTALL_SINGBOX=0",
		"--install-xray) INSTALL_XRAY=1; EXPLICIT_INSTALL_XRAY=1",
		"--install-singbox) INSTALL_SINGBOX=1; EXPLICIT_INSTALL_SINGBOX=1",
		"if [ \"$EXPLICIT_INSTALL_XRAY\" -eq 1 ]; then install_xray; else maybe_install_core \"Xray\" install_xray; fi",
		"if [ \"$EXPLICIT_INSTALL_SINGBOX\" -eq 1 ]; then install_singbox; else maybe_install_core \"sing-box\" install_singbox; fi",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer explicit core strictness contract missing %q", want)
		}
	}
}

func TestInstallerVerifiesXrayArchiveChecksumBeforeExtracting(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"xray_artifact=\"Xray-linux-${xray_asset_arch}.zip\"",
		"xray_dgst_url=\"${xray_url}.dgst\"",
		"curl -fL \"$xray_url\" -o \"$tmp_xray/$xray_artifact\"",
		"awk -F'= ' -v asset=\"$xray_artifact\" '/^SHA2-256=/{print $2 \"  \" asset}' \"$tmp_xray/$xray_artifact.dgst\" > \"$tmp_xray/$xray_artifact.sha256\"",
		"verify_sha256 \"$xray_artifact.sha256\" \"$tmp_xray\"",
		"unzip -oq \"$tmp_xray/$xray_artifact\" -d \"$tmp_xray/xray\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer Xray checksum contract missing %q", want)
		}
	}
	if strings.Index(script, "verify_sha256 \"$xray_artifact.sha256\" \"$tmp_xray\"") > strings.Index(script, "unzip -oq \"$tmp_xray/$xray_artifact\" -d \"$tmp_xray/xray\"") {
		t.Fatalf("installer must verify Xray checksum before extracting archive")
	}
}

func TestInstallerSkipsSingBoxSystemdUnitWhenSystemdUnavailable(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"if [ \"$SYSTEMD_AVAILABLE\" -ne 1 ]; then",
		"systemd 不可用，跳过 sing-box.service 写入。",
		"Manual run: /usr/local/bin/sing-box run -c /etc/sing-box/config.json",
		"cat > \"$SINGBOX_SERVICE_PATH\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer sing-box non-systemd contract missing %q", want)
		}
	}
	if strings.Index(script, "systemd 不可用，跳过 sing-box.service 写入。") > strings.Index(script, "cat > \"$SINGBOX_SERVICE_PATH\"") {
		t.Fatalf("installer must skip sing-box unit before writing service file when systemd is unavailable")
	}
}

func TestInstallerSupportsNonInteractiveUpdateMode(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"--upgrade|--update) ACTION=\"upgrade\"; SKIP_CORE_PROMPTS=1",
		"--check)",
		"--version)",
		"check_update()",
		"note_current_release_state",
		"normalize_version",
		"install_release_flow",
		"download_release_asset",
		"install_migate_binary_from_tmp",
		"systemctl restart migate",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer update contract missing %q", want)
		}
	}
}

func TestInstallerRepairServiceRefreshesSandboxPermissions(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"ReadWritePaths=${CONFIG_DIR} ${INSTALL_DIR} /var/log /etc/sing-box /etc/xray /usr/local/bin /usr/local/share/xray /usr/local/etc/xray /etc/systemd/system",
		"repair_service_flow()",
		"write_systemd_service",
		"restart_migate_service",
		"run_cmd systemctl daemon-reload",
		"run_cmd systemctl restart migate",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("repair-service cannot refresh sandbox permission %q", want)
		}
	}
	repairIdx := strings.Index(script, "repair_service_flow()")
	if repairIdx < 0 {
		t.Fatalf("repair-service must define repair_service_flow")
	}
	repairBody := script[repairIdx:]
	writeIdx := strings.Index(repairBody, "write_systemd_service")
	restartIdx := strings.Index(repairBody, "restart_migate_service")
	if writeIdx < 0 || restartIdx < 0 || writeIdx > restartIdx {
		t.Fatalf("repair-service must rewrite migate.service before restarting service")
	}
}

func TestInstallerUpdateFlowRewritesServiceBeforeRestart(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	writeIdx := strings.Index(script, "write_systemd_service")
	restartIdx := strings.Index(script, "section \"服务启动\"")
	if writeIdx < 0 || restartIdx < 0 || writeIdx > restartIdx {
		t.Fatalf("update/install flow must rewrite migate.service before service restart")
	}
	for _, want := range []string{
		"install_release_flow()",
		"write_systemd_service",
		"restart_migate_service",
		"run_cmd systemctl daemon-reload",
		"run_cmd systemctl restart migate",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("update flow service refresh contract missing %q", want)
		}
	}
}

func TestInstallerUpdateFlowRefreshesServiceEvenWhenCurrent(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	if strings.Contains(script, "if skip_update_if_current; then") {
		t.Fatalf("update flow must not return before service and installer self-repair when already current")
	}
	for _, want := range []string{
		"note_current_release_state",
		"MiGate 已是最新版本",
		"将刷新安装器和服务配置",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("update flow current-version repair contract missing %q", want)
		}
	}
	flowIdx := strings.Index(script, "install_release_flow()")
	if flowIdx < 0 {
		t.Fatalf("update flow must define install_release_flow")
	}
	flowBody := script[flowIdx:]
	noteIdx := strings.Index(flowBody, "note_current_release_state")
	downloadIdx := strings.Index(flowBody, "download_release_asset")
	installIdx := strings.Index(flowBody, "install_migate_binary_from_tmp")
	writeIdx := strings.Index(flowBody, "write_systemd_service")
	restartIdx := strings.Index(flowBody, "section \"服务启动\"")
	if noteIdx < 0 || downloadIdx < 0 || installIdx < 0 || writeIdx < 0 || restartIdx < 0 {
		t.Fatalf("update flow must include current-version note, release download, installer refresh, service rewrite, and restart")
	}
	if !(noteIdx < downloadIdx && downloadIdx < installIdx && installIdx < writeIdx && writeIdx < restartIdx) {
		t.Fatalf("update flow must continue from current-version note through installer/service refresh before restart")
	}
}

func TestInstallerUpdateSkipsCorePromptsUnlessExplicitlyRequested(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "packaging", "install.sh"), "--upgrade", "--yes", "--dry-run")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run upgrade failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		"未指定 --install-xray，跳过 Xray 安装。",
		"未指定 --install-singbox，跳过 sing-box 安装。",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dry-run upgrade missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{
		"[DRY-RUN] install /usr/local/bin/xray",
		"[DRY-RUN] install /usr/local/bin/sing-box",
		"确认安装/修复",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("dry-run upgrade must not run/prompt core install %q:\n%s", forbidden, text)
		}
	}
}

func TestInstallerReplacesRunningBinaryAtomicallyDuringUpdate(t *testing.T) {
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"local migate_tmp",
		"mktemp /usr/local/bin/.migate.XXXXXX",
		"cat \"$TMP/migate\" > \"$migate_tmp\"",
		"chmod +x \"$migate_tmp\"",
		"mv -f \"$migate_tmp\" \"$MIGATE_BIN\"",
		"ln -sf \"$MIGATE_BIN\" \"$MIGATE_LINK\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer atomic binary replacement contract missing %q", want)
		}
	}
	if strings.Contains(script, "cp \"$TMP/migate\" /usr/local/bin/migate") {
		t.Fatalf("installer must not cp over the running migate binary; use temp file plus atomic mv")
	}
}

func TestUninstallScriptStopsServicesAndRemovesInstalledArtifacts(t *testing.T) {
	script := read(t, "packaging", "uninstall.sh")
	for _, want := range []string{
		"DRY_RUN=0",
		"--dry-run",
		"run_cmd()",
		"[DRY-RUN]",
		"systemctl stop migate",
		"systemctl disable migate",
		"rm -f /etc/systemd/system/migate.service",
		"rm -f /usr/local/bin/migate",
		"rm -f /usr/local/bin/mg",
		"systemctl stop sing-box",
		"systemctl disable sing-box",
		"rm -f /etc/systemd/system/sing-box.service",
		"systemctl stop migate-singbox",
		"systemctl disable migate-singbox",
		"rm -f /etc/systemd/system/migate-singbox.service",
		"systemctl daemon-reload",
		"systemctl reset-failed",
		"--purge",
		"rm -rf /etc/migate",
		"rm -rf /usr/local/migate",
		"rm -rf /etc/sing-box",
		"rm -f /usr/local/etc/xray/config.json",
		"rm -f /usr/local/etc/xray/xray.json",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("uninstall script missing %q", want)
		}
	}

	if strings.Contains(strings.ToLower(script), "xray-install") {
		t.Fatalf("uninstall must not remove third-party Xray installation by default")
	}
}

func TestUninstallDryRunPrintsPlannedCommands(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "packaging", "uninstall.sh"), "--dry-run", "--yes")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("uninstall dry-run failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"[DRY-RUN] systemctl stop migate",
		"[DRY-RUN] rm -f /usr/local/bin/migate",
		"Keeping MiGate config/data",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("uninstall dry-run missing %q:\n%s", want, output)
		}
	}
}

func TestInstallerUninstallDryRunDelegatesWithoutEmptyArgument(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("bash", filepath.Join(root, "packaging", "install.sh"), "--uninstall", "--dry-run", "--yes")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("installer uninstall dry-run failed: %v\n%s", err, output)
	}
	out := string(output)
	for _, want := range []string{
		"[DRY-RUN] systemctl stop migate",
		"[DRY-RUN] rm -f /usr/local/bin/migate",
		"MiGate uninstalled.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("installer uninstall dry-run missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Unknown option") {
		t.Fatalf("installer passed an empty or invalid option to uninstaller:\n%s", out)
	}
}

func TestReleaseArchivesIncludeUninstallScript(t *testing.T) {
	root := repoRoot(t)
	distDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "packaging", "build-release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "DIST_DIR="+distDir, "VERSION=v0.0.0-test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release failed: %v\n%s", err, output)
	}
	for _, artifact := range []string{"migate-linux-amd64.tar.gz", "migate-linux-arm64.tar.gz"} {
		entries := tarEntries(t, filepath.Join(distDir, artifact))
		if !entries["packaging/uninstall.sh"] {
			t.Fatalf("%s missing packaging/uninstall.sh, entries=%v", artifact, entries)
		}
	}
}

func TestServiceUsesGeneratedPanelConfigAndSingleBinary(t *testing.T) {
	service := read(t, "packaging", "migate.service")
	script := read(t, "packaging", "install.sh")
	for _, want := range []string{
		"ExecStart=/usr/local/bin/migate serve",
		"--host 0.0.0.0",
		"--config /etc/migate/panel.json",
		"Restart=on-failure",
		"NoNewPrivileges=true",
		"PrivateTmp=true",
		"ProtectSystem=strict",
		"ReadWritePaths=/etc/migate /usr/local/migate /var/log /etc/sing-box /etc/xray /usr/local/bin /usr/local/share/xray /usr/local/etc/xray /etc/systemd/system",
		"CapabilityBoundingSet=CAP_NET_BIND_SERVICE",
		"RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("service missing %q: %s", want, service)
		}
	}
	for _, want := range []string{
		`mkdir -p "$CONFIG_DIR" "$INSTALL_DIR" /etc/sing-box /etc/xray /usr/local/bin /usr/local/share/xray /usr/local/etc/xray /etc/systemd/system`,
		"ProtectSystem=strict",
		"ReadWritePaths=${CONFIG_DIR} ${INSTALL_DIR} /var/log /etc/sing-box /etc/xray /usr/local/bin /usr/local/share/xray /usr/local/etc/xray /etc/systemd/system",
		"CapabilityBoundingSet=CAP_NET_BIND_SERVICE",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer-generated service missing permission contract %q", want)
		}
	}
	for _, forbidden := range []string{
		"ProtectHome=",
		"SystemCallFilter=",
	} {
		if strings.Contains(service, forbidden) {
			t.Fatalf("service must not restrict runtime management permissions with %q: %s", forbidden, service)
		}
		if strings.Contains(script, forbidden) {
			t.Fatalf("installer-generated service must not restrict runtime management permissions with %q", forbidden)
		}
	}
	forbidden := []string{"python", "uv", "pip", "npm", join("open", "vpn"), "tun", "egress", "remote", "leak", "rollout"}
	lower := strings.ToLower(service)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("service must not contain %q: %s", word, service)
		}
	}
}

func TestBuildReleaseScriptProducesLinuxArchivesAndChecksums(t *testing.T) {
	root := repoRoot(t)
	distDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "packaging", "build-release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "DIST_DIR="+distDir, "VERSION=v0.0.0-test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build release failed: %v\n%s", err, output)
	}

	for _, artifact := range []string{"migate-linux-amd64.tar.gz", "migate-linux-arm64.tar.gz", "checksums.txt"} {
		path := filepath.Join(distDir, artifact)
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("expected non-empty artifact %s, stat=%v info=%+v\noutput:\n%s", artifact, err, info, output)
		}
	}

	checksums := mustReadFile(t, filepath.Join(distDir, "checksums.txt"))
	for _, artifact := range []string{"migate-linux-amd64.tar.gz", "migate-linux-arm64.tar.gz"} {
		if !strings.Contains(checksums, artifact) {
			t.Fatalf("checksums missing %s: %s", artifact, checksums)
		}
		entries := tarEntries(t, filepath.Join(distDir, artifact))
		for _, want := range []string{"migate", "packaging/migate.service", "packaging/install.sh"} {
			if !entries[want] {
				t.Fatalf("%s missing %s, entries=%v", artifact, want, entries)
			}
		}
		forbidden := []string{".git/", "node_modules/", "python", join("open", "vpn"), "rollout", "leak", "egress"}
		for name := range entries {
			lower := strings.ToLower(name)
			for _, word := range forbidden {
				if strings.Contains(lower, word) {
					t.Fatalf("%s contains forbidden release entry %q", artifact, name)
				}
			}
		}
	}
}

func TestBuildReleaseScriptStripsReleaseBinaries(t *testing.T) {
	script := read(t, "packaging", "build-release.sh")
	if !strings.Contains(script, "-ldflags \"-s -w -X main.Version=${VERSION}\"") {
		t.Fatalf("release build must strip symbols/debug info with -s -w: %s", script)
	}
}

func TestReleaseWorkflowBuildsAndUploadsReleaseAssets(t *testing.T) {
	workflow := read(t, ".github", "workflows", "release.yml")
	for _, want := range []string{
		"name: Release",
		"push:",
		"tags:",
		"v*",
		"contents: write",
		"actions/checkout",
		"actions/setup-go",
		"go-version-file: go.mod",
		"actions/setup-node",
		"node-version: 24",
		"cache: npm",
		"cache-dependency-path: web/package-lock.json",
		"packaging/build-release.sh",
		"sha256sum -c checksums.txt",
		"softprops/action-gh-release",
		"dist/migate-linux-amd64.tar.gz",
		"dist/migate-linux-arm64.tar.gz",
		"dist/checksums.txt",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("release workflow missing %q", want)
		}
	}

	forbidden := []string{"npm install", "npm run", "node_modules", "pip", "uv ", "python", join("open", "vpn"), "rollout", "leak", "egress"}
	lower := strings.ToLower(workflow)
	for _, word := range forbidden {
		if strings.Contains(lower, word) {
			t.Fatalf("release workflow must not contain %q", word)
		}
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func tarEntries(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open archive %s: %v", path, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader %s: %v", path, err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	entries := map[string]bool{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read tar %s: %v", path, err)
		}
		entries[header.Name] = true
	}
	return entries
}
