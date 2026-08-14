package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 为控制器配置。
// 说明：字段与 config/controller.yaml 对应，便于运维直接修改。
type Config struct {
	ListenAddr string `yaml:"listen_addr"`
	// InternalListenAddr 为内部接口监听地址（建议绑定内网 IP + 独立端口）。
	// 为空时不启用内部独立监听。
	InternalListenAddr string `yaml:"internal_listen_addr"`
	// InternalTLSCertFile / InternalTLSKeyFile 为内部监听 HTTPS 证书与私钥路径。
	InternalTLSCertFile string `yaml:"internal_tls_cert_file"`
	InternalTLSKeyFile  string `yaml:"internal_tls_key_file"`

	DatabaseDSN string `yaml:"database_dsn"`

	AgentToken            string            `yaml:"agent_token"`
	AgentLegacyTokens     []string          `yaml:"agent_legacy_tokens"`
	AgentNodeTokens       map[string]string `yaml:"agent_node_tokens"`
	AgentNodeTokenEnforce bool              `yaml:"agent_node_token_enforce"`
	AdminToken            string            `yaml:"admin_token"`
	// AuthSecret 用于签名 Web 登录会话（cookie）。建议使用强随机值。
	AuthSecret string `yaml:"auth_secret"`

	WarningThreshold float64 `yaml:"warning_threshold"`
	LimitedThreshold float64 `yaml:"limited_threshold"`

	CPUPricePerCoreMinute float64 `yaml:"cpu_price_per_core_minute"`
	SampleIntervalSeconds int     `yaml:"sample_interval_seconds"`

	EnableCPUControl       bool    `yaml:"enable_cpu_control"`
	CPULimitPercentLimited float64 `yaml:"cpu_limit_percent_limited"`
	CPULimitPercentBlocked float64 `yaml:"cpu_limit_percent_blocked"`
	OverdraftMemoryLimitGB float64 `yaml:"overdraft_memory_limit_gb"`

	KillGracePeriodSeconds int `yaml:"kill_grace_period_seconds"`

	DryRun bool `yaml:"dry_run"`

	DefaultBalance        float64 `yaml:"default_balance"`
	DefaultPricePerMinute float64 `yaml:"default_price_per_minute"`

	// Web 登录会话配置
	SessionHours int  `yaml:"session_hours"`
	CookieSecure bool `yaml:"cookie_secure"`
	// TurnstileSiteKey / TurnstileSecretKey 同时配置后启用 Cloudflare Turnstile。
	// TurnstileExpectedHostnames 用于校验 Siteverify 返回的 hostname，避免 token 被跨站复用。
	TurnstileSiteKey           string   `yaml:"turnstile_site_key"`
	TurnstileSecretKey         string   `yaml:"turnstile_secret_key"`
	TurnstileExpectedHostnames []string `yaml:"turnstile_expected_hostnames"`

	// 邮件找回密码默认配置（可在管理员页面里在线修改）
	SMTPHost  string `yaml:"smtp_host"`
	SMTPPort  int    `yaml:"smtp_port"`
	SMTPUser  string `yaml:"smtp_user"`
	SMTPPass  string `yaml:"smtp_pass"`
	FromEmail string `yaml:"from_email"`
	FromName  string `yaml:"from_name"`

	MigrationDir string `yaml:"migration_dir"`
	WebDir       string `yaml:"web_dir"`

	// 容灾配置（主备控制器）
	HAEnabled bool   `yaml:"ha_enabled"`
	HANode    string `yaml:"ha_node"`
	HARole    string `yaml:"ha_role"` // primary / standby
	HAPeerURL string `yaml:"ha_peer_url"`
	HAToken   string `yaml:"ha_token"`

	// BackupStatusFile 由系统级备份任务写入，控制器只读并在管理员页面展示。
	BackupStatusFile       string `yaml:"backup_status_file"`
	BackupVerifyStatusFile string `yaml:"backup_verify_status_file"`

	// NFS 共享工作目录（控制器本机直接管理服务端真实路径，避免计算节点 NFS root_squash 导致 mkdir/chown 失败）
	SharedNodeRoot    string `yaml:"shared_node_root"`
	SharedClusterRoot string `yaml:"shared_cluster_root"`
}

func (c *Config) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("listen_addr 不能为空")
	}
	internalAddr := strings.TrimSpace(c.InternalListenAddr)
	internalCert := strings.TrimSpace(c.InternalTLSCertFile)
	internalKey := strings.TrimSpace(c.InternalTLSKeyFile)
	if internalAddr == "" && (internalCert != "" || internalKey != "") {
		return errors.New("配置 internal_tls_cert_file/internal_tls_key_file 时 internal_listen_addr 不能为空")
	}
	if internalAddr != "" {
		if internalCert == "" || internalKey == "" {
			return errors.New("启用 internal_listen_addr 时必须配置 internal_tls_cert_file 与 internal_tls_key_file")
		}
	}
	if c.DatabaseDSN == "" {
		return errors.New("database_dsn 不能为空")
	}
	if c.WarningThreshold <= 0 {
		return errors.New("warning_threshold 必须为正数")
	}
	if c.LimitedThreshold < 0 || c.LimitedThreshold >= c.WarningThreshold {
		return errors.New("limited_threshold 必须在 [0, warning_threshold) 范围内")
	}
	if c.CPUPricePerCoreMinute < 0 {
		return errors.New("cpu_price_per_core_minute 不能为负数")
	}
	if c.SampleIntervalSeconds <= 0 || c.SampleIntervalSeconds > 600 {
		return errors.New("sample_interval_seconds 必须在 (0, 600] 范围内")
	}
	if c.CPULimitPercentLimited < 1 || c.CPULimitPercentLimited > 100 {
		return errors.New("cpu_limit_percent_limited 必须在 [1, 100] 范围内")
	}
	if c.CPULimitPercentBlocked < 1 || c.CPULimitPercentBlocked > 100 {
		return errors.New("cpu_limit_percent_blocked 必须在 [1, 100] 范围内")
	}
	if c.OverdraftMemoryLimitGB < 0 {
		return errors.New("overdraft_memory_limit_gb 不能为负数（0 表示关闭）")
	}
	if c.KillGracePeriodSeconds < 0 {
		return errors.New("kill_grace_period_seconds 不能为负数")
	}
	if c.DefaultBalance < 0 {
		return errors.New("default_balance 不能为负数")
	}
	if c.DefaultPricePerMinute < 0 {
		return errors.New("default_price_per_minute 不能为负数")
	}
	if c.AgentToken == "" {
		return errors.New("agent_token 不能为空（用于保护 /api/metrics）")
	}
	for _, tok := range c.AgentLegacyTokens {
		if strings.TrimSpace(tok) == "" {
			return errors.New("agent_legacy_tokens 不能包含空值")
		}
	}
	for nodeID, tok := range c.AgentNodeTokens {
		if strings.TrimSpace(nodeID) == "" {
			return errors.New("agent_node_tokens 不能包含空 node_id")
		}
		if strings.TrimSpace(tok) == "" {
			return errors.New("agent_node_tokens 不能包含空 token")
		}
	}
	if c.AdminToken == "" {
		return errors.New("admin_token 不能为空（用于保护 /api/admin/* 与充值/单价设置）")
	}
	if c.SessionHours < 0 || c.SessionHours > 720 {
		return errors.New("session_hours 必须在 [0, 720]（0 表示禁用会话）")
	}
	// 禁止任何“隐式降级”：只要启用了 Web 登录会话，就必须显式配置 auth_secret。
	if c.SessionHours > 0 {
		if c.AuthSecret == "" {
			return errors.New("启用 Web 登录会话时 auth_secret 不能为空（建议 openssl rand -hex 32）")
		}
		if len(c.AuthSecret) < 16 {
			return errors.New("auth_secret 长度过短：建议至少 16 字符（建议 openssl rand -hex 32）")
		}
	}
	turnstileSiteKey := strings.TrimSpace(c.TurnstileSiteKey)
	turnstileSecretKey := strings.TrimSpace(c.TurnstileSecretKey)
	if (turnstileSiteKey == "") != (turnstileSecretKey == "") {
		return errors.New("turnstile_site_key 与 turnstile_secret_key 必须同时配置或同时留空")
	}
	if turnstileSiteKey != "" {
		if len(c.TurnstileExpectedHostnames) == 0 {
			return errors.New("启用 Turnstile 时 turnstile_expected_hostnames 不能为空")
		}
		seenHostnames := make(map[string]struct{}, len(c.TurnstileExpectedHostnames))
		hostnames := make([]string, 0, len(c.TurnstileExpectedHostnames))
		for _, raw := range c.TurnstileExpectedHostnames {
			hostname := strings.ToLower(strings.TrimSpace(raw))
			if hostname == "" {
				return errors.New("turnstile_expected_hostnames 不能包含空值")
			}
			if strings.ContainsAny(hostname, "/:") {
				return errors.New("turnstile_expected_hostnames 只能填写 hostname 或 IPv4，不能包含协议、端口或路径")
			}
			if _, exists := seenHostnames[hostname]; exists {
				continue
			}
			seenHostnames[hostname] = struct{}{}
			hostnames = append(hostnames, hostname)
		}
		c.TurnstileExpectedHostnames = hostnames
	}
	if c.SMTPPort < 0 || c.SMTPPort > 65535 {
		return errors.New("smtp_port 必须在 [0, 65535]")
	}
	if c.HAEnabled {
		role := strings.TrimSpace(strings.ToLower(c.HARole))
		if role != "" && role != "primary" && role != "standby" {
			return errors.New("ha_role 仅支持 primary/standby")
		}
	}
	if strings.TrimSpace(c.BackupStatusFile) == "" {
		c.BackupStatusFile = "/var/lib/gpu-controller/backup-status.json"
	}
	if strings.TrimSpace(c.BackupVerifyStatusFile) == "" {
		c.BackupVerifyStatusFile = "/var/lib/gpu-controller/backup-verify-status.json"
	}
	if strings.TrimSpace(c.SharedNodeRoot) == "" {
		c.SharedNodeRoot = filepath.FromSlash("/srv/gpu-ops/nodes")
	}
	if strings.TrimSpace(c.SharedClusterRoot) == "" {
		c.SharedClusterRoot = filepath.FromSlash("/srv/gpu-ops/cluster")
	}
	return nil
}

type cliArgs struct {
	configPath string
	showVer    bool
}

func parseArgs() cliArgs {
	var a cliArgs
	flag.StringVar(&a.configPath, "config", "", "配置文件路径（yaml）")
	flag.BoolVar(&a.showVer, "version", false, "打印版本信息并退出")
	flag.Parse()
	return a
}

func defaultConfigPath() (string, error) {
	if p := os.Getenv("CONTROLLER_CONFIG"); p != "" {
		if fileExists(p) {
			return p, nil
		}
		return "", fmt.Errorf("环境变量 CONTROLLER_CONFIG 指向的文件不存在：%s", p)
	}

	candidates := []string{
		filepath.FromSlash("../config/controller.yaml"),
		filepath.FromSlash("config/controller.yaml"),
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p, nil
		}
	}
	return "", errors.New("未找到默认配置文件：请使用 --config 或设置 CONTROLLER_CONFIG")
}

func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
