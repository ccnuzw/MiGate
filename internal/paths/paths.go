package paths

var (
	ConfigDir       = "/etc/migate"
	CoreConfigDir   = "/etc/migate/cores"
	PanelConfig     = "/etc/migate/panel.json"
	XrayConfig      = "/etc/migate/cores/xray.json"
	SingboxConfig   = "/etc/migate/cores/sing-box.json"
	DataDir         = "/var/lib/migate"
	Database        = "/var/lib/migate/migate.db"
	BackupDir       = "/var/lib/migate/backups"
	VersionsFile    = "/var/lib/migate/versions.json"
	LogDir          = "/var/log/migate"
	RunDir          = "/run/migate"
	BinDir          = "/usr/local/bin"
	XrayBinary      = "/usr/local/bin/xray"
	SingboxBinary   = "/usr/local/bin/sing-box"
	MigateBinary    = "/usr/local/bin/migate"
	MigateCLI       = "/usr/local/bin/mg"
	Installer       = "/usr/local/bin/migate-install"
	Uninstaller     = "/usr/local/bin/migate-uninstall"
	DefaultHTTPPort = 9999
)

const (
	PanelService   = "migate"
	XrayService    = "migate-xray"
	SingboxService = "migate-sing-box"

	PanelServiceUnit   = "migate.service"
	XrayServiceUnit    = "migate-xray.service"
	SingboxServiceUnit = "migate-sing-box.service"
)

var (
	InstallLock = "/run/migate/install.lock"
	ApplyLock   = "/run/migate/apply.lock"
	SyncLock    = "/run/migate/sync.lock"
	UpgradeLock = "/run/migate/upgrade.lock"
)
