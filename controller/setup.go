package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

const (
	defaultPlatformName                       = "Bursar"
	appSettingPlatformName                    = "platform_name"
	appSettingRegistrationAllowedEmailDomains = "registration_allowed_email_domains"
	appSettingProvisionSSHHost                = "provision_ssh_host"
	appSettingSetupCompleted                  = "setup_completed"
)

type PlatformSettings struct {
	PlatformName                    string   `json:"platform_name"`
	RegistrationAllowedEmailDomains []string `json:"registration_allowed_email_domains"`
	ProvisionSSHHost                string   `json:"provision_ssh_host"`
	SetupCompleted                  bool     `json:"setup_completed"`
}

type SetupMailSettings struct {
	Enabled         bool   `json:"enabled"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPass        string `json:"smtp_pass,omitempty"`
	SMTPPasswordSet bool   `json:"smtp_password_set"`
	FromEmail       string `json:"from_email"`
	FromName        string `json:"from_name"`
}

type SetupStartupCheck struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	OK       bool   `json:"ok"`
	Required bool   `json:"required"`
	Message  string `json:"message"`
}

type SetupStartupConfig struct {
	ListenAddr         string `json:"listen_addr"`
	InternalListenAddr string `json:"internal_listen_addr"`
	SharedNodeRoot     string `json:"shared_node_root"`
	SharedClusterRoot  string `json:"shared_cluster_root"`
}

type AdminSetupState struct {
	Platform      PlatformSettings    `json:"platform"`
	UserGuideline string              `json:"user_guideline"`
	Prices        []PriceRow          `json:"prices"`
	Mail          SetupMailSettings   `json:"mail"`
	HA            HASyncConfig        `json:"ha"`
	Startup       SetupStartupConfig  `json:"startup"`
	Checks        []SetupStartupCheck `json:"checks"`
}

type adminSetupSaveReq struct {
	Platform      PlatformSettings  `json:"platform"`
	UserGuideline string            `json:"user_guideline"`
	Prices        []PriceRow        `json:"prices"`
	Mail          SetupMailSettings `json:"mail"`
	HA            HASyncConfig      `json:"ha"`
}

func defaultPlatformSettings() PlatformSettings {
	return PlatformSettings{
		PlatformName:                    defaultPlatformName,
		RegistrationAllowedEmailDomains: []string{},
		ProvisionSSHHost:                "",
		SetupCompleted:                  false,
	}
}

func normalizeEmailDomains(domains []string) ([]string, error) {
	seen := make(map[string]struct{}, len(domains))
	out := make([]string, 0, len(domains))
	for _, raw := range domains {
		domain := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "@")))
		if domain == "" {
			continue
		}
		if strings.ContainsAny(domain, " /:@") {
			return nil, fmt.Errorf("邮箱域名格式不合法：%s", raw)
		}
		if _, err := mail.ParseAddress("setup@" + domain); err != nil || !strings.Contains(domain, ".") {
			return nil, fmt.Errorf("邮箱域名格式不合法：%s", raw)
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeProvisionSSHHost(raw string) (string, error) {
	host := strings.TrimSpace(raw)
	if host == "" {
		return "", nil
	}
	if strings.Contains(host, "://") {
		return "", errors.New("SSH 主机只能填写主机名或 IP，不能包含协议、端口或路径")
	}
	if strings.ContainsAny(host, " /@") {
		return "", errors.New("SSH 主机只能填写主机名或 IP，不能包含用户、端口或路径")
	}
	if ip := net.ParseIP(host); ip == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 {
				return "", errors.New("SSH 主机格式不合法")
			}
			for _, r := range label {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
					return "", errors.New("SSH 主机格式不合法")
				}
			}
		}
	}
	return host, nil
}

func (s *Store) GetPlatformSettings(ctx context.Context) (PlatformSettings, error) {
	out := defaultPlatformSettings()
	rows, err := s.db.QueryContext(ctx, `
SELECT key, value
FROM app_settings
WHERE key = ANY($1)`, pq.Array([]string{
		appSettingPlatformName,
		appSettingRegistrationAllowedEmailDomains,
		appSettingProvisionSSHHost,
		appSettingSetupCompleted,
	}))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return out, err
		}
		switch key {
		case appSettingPlatformName:
			if v := strings.TrimSpace(value); v != "" {
				out.PlatformName = v
			}
		case appSettingRegistrationAllowedEmailDomains:
			var domains []string
			if err := json.Unmarshal([]byte(value), &domains); err == nil {
				if normalized, err := normalizeEmailDomains(domains); err == nil {
					out.RegistrationAllowedEmailDomains = normalized
				}
			}
		case appSettingProvisionSSHHost:
			out.ProvisionSSHHost = strings.TrimSpace(value)
		case appSettingSetupCompleted:
			out.SetupCompleted = parseSettingBool(value, false)
		}
	}
	return out, rows.Err()
}

func upsertAppSettingTx(ctx context.Context, tx *sql.Tx, key, value string) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO app_settings(key, value, updated_at)
VALUES($1,$2,NOW())
ON CONFLICT (key) DO UPDATE
SET value=EXCLUDED.value, updated_at=NOW()`, key, value)
	return err
}

func (s *Store) SavePlatformSettings(ctx context.Context, settings PlatformSettings) error {
	settings.PlatformName = strings.TrimSpace(settings.PlatformName)
	if settings.PlatformName == "" || len([]rune(settings.PlatformName)) > 80 {
		return errors.New("平台名称不能为空且不能超过 80 个字符")
	}
	domains, err := normalizeEmailDomains(settings.RegistrationAllowedEmailDomains)
	if err != nil {
		return err
	}
	sshHost, err := normalizeProvisionSSHHost(settings.ProvisionSSHHost)
	if err != nil {
		return err
	}
	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	items := map[string]string{
		appSettingPlatformName:                    settings.PlatformName,
		appSettingRegistrationAllowedEmailDomains: string(domainsJSON),
		appSettingProvisionSSHHost:                sshHost,
		appSettingSetupCompleted:                  strconv.FormatBool(settings.SetupCompleted),
	}
	for key, value := range items {
		if err := upsertAppSettingTx(ctx, tx, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DisableMailSettings(ctx context.Context, fromName string) error {
	fromName = strings.TrimSpace(fromName)
	if fromName == "" {
		fromName = defaultPlatformName
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	items := map[string]string{
		appSettingSMTPHost:  "",
		appSettingSMTPPort:  "587",
		appSettingSMTPUser:  "",
		appSettingSMTPPass:  "",
		appSettingFromEmail: "",
		appSettingFromName:  fromName,
	}
	for key, value := range items {
		if err := upsertAppSettingTx(ctx, tx, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func isConfiguredSecret(value string, minLen int) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if len(strings.TrimSpace(value)) < minLen {
		return false
	}
	return !strings.Contains(v, "replace-with") && !strings.Contains(v, "change-me") && !strings.Contains(v, "example")
}

func (s *Server) setupStartupChecks() []SetupStartupCheck {
	internalTLSOK := strings.TrimSpace(s.cfg.InternalListenAddr) == "" ||
		(strings.TrimSpace(s.cfg.InternalTLSCertFile) != "" && strings.TrimSpace(s.cfg.InternalTLSKeyFile) != "")
	return []SetupStartupCheck{
		{Key: "database", Label: "数据库连接", OK: s.store != nil, Required: true, Message: "控制器已连接 PostgreSQL 并可读取设置。"},
		{Key: "auth_secret", Label: "会话签名密钥", OK: isConfiguredSecret(s.cfg.AuthSecret, 16), Required: true, Message: "在 controller.yaml 中配置至少 16 位随机 auth_secret。"},
		{Key: "admin_token", Label: "管理员 API Token", OK: isConfiguredSecret(s.cfg.AdminToken, 16), Required: true, Message: "在 controller.yaml 中配置独立的强随机 admin_token。"},
		{Key: "agent_token", Label: "节点 Agent Token", OK: isConfiguredSecret(s.cfg.AgentToken, 16), Required: true, Message: "在 controller.yaml 中配置独立的强随机 agent_token。"},
		{Key: "internal_tls", Label: "内部接口 TLS", OK: internalTLSOK, Required: false, Message: "启用内部监听时必须同时配置证书和私钥。"},
	}
}

func (s *Server) loadAdminSetupState(ctx context.Context) (AdminSetupState, error) {
	platform, err := s.store.GetPlatformSettings(ctx)
	if err != nil {
		return AdminSetupState{}, err
	}
	guideline, _, _, err := s.store.GetUserGuideline(ctx)
	if err != nil {
		return AdminSetupState{}, err
	}
	prices, err := s.store.ListPrices(ctx)
	if err != nil {
		return AdminSetupState{}, err
	}
	mailSettings, err := s.store.GetMailSettings(ctx, s.cfg)
	if err != nil {
		return AdminSetupState{}, err
	}
	ha, err := s.store.GetHASyncConfig(ctx, s.cfg)
	if err != nil {
		return AdminSetupState{}, err
	}
	return AdminSetupState{
		Platform:      platform,
		UserGuideline: guideline,
		Prices:        prices,
		Mail: SetupMailSettings{
			Enabled:         strings.TrimSpace(mailSettings.SMTPUser) != "",
			SMTPHost:        mailSettings.SMTPHost,
			SMTPPort:        mailSettings.SMTPPort,
			SMTPUser:        mailSettings.SMTPUser,
			SMTPPasswordSet: strings.TrimSpace(mailSettings.SMTPPass) != "",
			FromEmail:       mailSettings.FromEmail,
			FromName:        mailSettings.FromName,
		},
		HA: ha,
		Startup: SetupStartupConfig{
			ListenAddr:         s.cfg.ListenAddr,
			InternalListenAddr: s.cfg.InternalListenAddr,
			SharedNodeRoot:     s.cfg.SharedNodeRoot,
			SharedClusterRoot:  s.cfg.SharedClusterRoot,
		},
		Checks: s.setupStartupChecks(),
	}, nil
}

func (s *Server) handlePublicSettings(c *gin.Context) {
	settings, err := s.store.GetPlatformSettings(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, settings)
}

func (s *Server) handleAdminSetupGet(c *gin.Context) {
	state, err := s.loadAdminSetupState(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, state)
}

func (s *Server) handleAdminSetupSave(c *gin.Context) {
	var req adminSetupSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	for _, check := range s.setupStartupChecks() {
		if check.Required && !check.OK {
			c.JSON(400, gin.H{"error": "请先完成启动配置中的必填安全项：" + check.Label})
			return
		}
	}
	req.Platform.SetupCompleted = false
	if err := s.store.SavePlatformSettings(c.Request.Context(), req.Platform); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertUserGuideline(c.Request.Context(), req.UserGuideline, c.GetString("auth_user")); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	for _, price := range req.Prices {
		if err := s.store.UpsertPrice(c.Request.Context(), price.Model, price.Price); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
	}
	if req.Mail.Enabled {
		mailSettings := MailSettings{
			SMTPHost:  req.Mail.SMTPHost,
			SMTPPort:  req.Mail.SMTPPort,
			SMTPUser:  req.Mail.SMTPUser,
			SMTPPass:  req.Mail.SMTPPass,
			FromEmail: req.Mail.FromEmail,
			FromName:  req.Mail.FromName,
		}
		updatePassword := strings.TrimSpace(req.Mail.SMTPPass) != ""
		if !req.Mail.SMTPPasswordSet && !updatePassword {
			c.JSON(400, gin.H{"error": "启用邮件通知时必须填写 SMTP 密码"})
			return
		}
		if err := s.store.UpsertMailSettings(c.Request.Context(), mailSettings, updatePassword); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
	} else if err := s.store.DisableMailSettings(c.Request.Context(), req.Platform.PlatformName); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if err := s.store.UpsertHASyncConfig(c.Request.Context(), req.HA); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	req.Platform.SetupCompleted = true
	if err := s.store.SavePlatformSettings(c.Request.Context(), req.Platform); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	state, err := s.loadAdminSetupState(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "setup": state})
}

func (s *Server) platformName(ctx context.Context) string {
	settings, err := s.store.GetPlatformSettings(ctx)
	if err != nil || strings.TrimSpace(settings.PlatformName) == "" {
		return defaultPlatformName
	}
	return settings.PlatformName
}
