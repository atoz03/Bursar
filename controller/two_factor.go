package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type TwoFactorState struct {
	Username     string `json:"username"`
	Role         string `json:"role"`
	Enabled      bool   `json:"enabled"`
	PendingSetup bool   `json:"pending_setup"`
	Issuer       string `json:"issuer"`
	AccountName  string `json:"account_name"`
}

type TwoFactorSetup struct {
	Username    string `json:"username"`
	Role        string `json:"role"`
	Enabled     bool   `json:"enabled"`
	Issuer      string `json:"issuer"`
	AccountName string `json:"account_name"`
	Secret      string `json:"secret"`
	OTPAUTHURL  string `json:"otpauth_url"`
}

type authAccountTwoFactorState struct {
	Enabled       bool
	Secret        string
	PendingSecret string
}

type authTwoFactorEnableReq struct {
	Code string `json:"code"`
}

type authTwoFactorDisableReq struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

type adminUserTwoFactorReq struct {
	Role string `json:"role"`
}

func normalizeAuthAccountRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "admin", "power_user", "user":
		return role
	default:
		return ""
	}
}

func authAccountTable(role string) (string, error) {
	switch normalizeAuthAccountRole(role) {
	case "admin":
		return "admin_accounts", nil
	case "power_user":
		return "power_users", nil
	case "user":
		return "user_accounts", nil
	default:
		return "", errors.New("role 不合法")
	}
}

func (s *Store) getAuthAccountTwoFactorState(ctx context.Context, username string, role string) (authAccountTwoFactorState, error) {
	username = strings.TrimSpace(username)
	table, err := authAccountTable(role)
	if err != nil {
		return authAccountTwoFactorState{}, err
	}
	if username == "" {
		return authAccountTwoFactorState{}, errors.New("username 不能为空")
	}
	query := fmt.Sprintf(`
SELECT COALESCE(two_factor_enabled, FALSE),
       COALESCE(two_factor_secret, ''),
       COALESCE(two_factor_pending_secret, '')
FROM %s
WHERE username=$1`, table)
	var out authAccountTwoFactorState
	if err := s.db.QueryRowContext(ctx, query, username).Scan(&out.Enabled, &out.Secret, &out.PendingSecret); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authAccountTwoFactorState{}, errors.New("账号不存在")
		}
		return authAccountTwoFactorState{}, err
	}
	return out, nil
}

func (s *Store) GetAccountTwoFactorState(ctx context.Context, username string, role string) (TwoFactorState, error) {
	state, err := s.getAuthAccountTwoFactorState(ctx, username, role)
	if err != nil {
		return TwoFactorState{}, err
	}
	role = normalizeAuthAccountRole(role)
	return TwoFactorState{
		Username:     strings.TrimSpace(username),
		Role:         role,
		Enabled:      state.Enabled,
		PendingSetup: strings.TrimSpace(state.PendingSecret) != "",
		Issuer:       totpIssuer,
		AccountName:  totpAccountName(role, username),
	}, nil
}

func (s *Store) BeginAccountTwoFactorSetup(ctx context.Context, username string, role string) (TwoFactorSetup, error) {
	username = strings.TrimSpace(username)
	role = normalizeAuthAccountRole(role)
	table, err := authAccountTable(role)
	if err != nil {
		return TwoFactorSetup{}, err
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		return TwoFactorSetup{}, err
	}
	query := fmt.Sprintf(`
UPDATE %s
SET two_factor_pending_secret=$2,
    updated_at=NOW()
WHERE username=$1
RETURNING username`, table)
	var returned string
	if err := s.db.QueryRowContext(ctx, query, username, secret).Scan(&returned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TwoFactorSetup{}, errors.New("账号不存在")
		}
		return TwoFactorSetup{}, err
	}
	return TwoFactorSetup{
		Username:    returned,
		Role:        role,
		Enabled:     false,
		Issuer:      totpIssuer,
		AccountName: totpAccountName(role, returned),
		Secret:      secret,
		OTPAUTHURL:  buildTOTPKeyURI(role, returned, secret),
	}, nil
}

func (s *Store) EnableAccountTwoFactorFromPending(ctx context.Context, username string, role string) error {
	username = strings.TrimSpace(username)
	table, err := authAccountTable(role)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
UPDATE %s
SET two_factor_enabled=TRUE,
    two_factor_secret=COALESCE(two_factor_pending_secret, ''),
    two_factor_pending_secret='',
    updated_at=NOW()
WHERE username=$1
  AND COALESCE(two_factor_pending_secret, '') <> ''
RETURNING username`, table)
	var returned string
	if err := s.db.QueryRowContext(ctx, query, username).Scan(&returned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("请先开始 2FA 绑定")
		}
		return err
	}
	return nil
}

func (s *Store) ForceEnableAccountTwoFactor(ctx context.Context, username string, role string) (TwoFactorSetup, error) {
	username = strings.TrimSpace(username)
	role = normalizeAuthAccountRole(role)
	table, err := authAccountTable(role)
	if err != nil {
		return TwoFactorSetup{}, err
	}
	secret, err := generateTOTPSecret()
	if err != nil {
		return TwoFactorSetup{}, err
	}
	query := fmt.Sprintf(`
UPDATE %s
SET two_factor_enabled=TRUE,
    two_factor_secret=$2,
    two_factor_pending_secret='',
    updated_at=NOW()
WHERE username=$1
RETURNING username`, table)
	var returned string
	if err := s.db.QueryRowContext(ctx, query, username, secret).Scan(&returned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TwoFactorSetup{}, errors.New("账号不存在")
		}
		return TwoFactorSetup{}, err
	}
	return TwoFactorSetup{
		Username:    returned,
		Role:        role,
		Enabled:     true,
		Issuer:      totpIssuer,
		AccountName: totpAccountName(role, returned),
		Secret:      secret,
		OTPAUTHURL:  buildTOTPKeyURI(role, returned, secret),
	}, nil
}

func (s *Store) DisableAccountTwoFactor(ctx context.Context, username string, role string) error {
	username = strings.TrimSpace(username)
	table, err := authAccountTable(role)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
UPDATE %s
SET two_factor_enabled=FALSE,
    two_factor_secret='',
    two_factor_pending_secret='',
    updated_at=NOW()
WHERE username=$1
RETURNING username`, table)
	var returned string
	if err := s.db.QueryRowContext(ctx, query, username).Scan(&returned); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("账号不存在")
		}
		return err
	}
	return nil
}

func (s *Store) RecordSuccessfulLogin(ctx context.Context, username string, role string, now time.Time) error {
	username = strings.TrimSpace(username)
	table, err := authAccountTable(role)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
UPDATE %s
SET last_login_at=$2,
    updated_at=NOW()
WHERE username=$1`, table)
	_, err = s.db.ExecContext(ctx, query, username, now)
	return err
}

func (s *Store) VerifyPasswordForRole(ctx context.Context, username string, role string, password string) (bool, error) {
	switch normalizeAuthAccountRole(role) {
	case "admin":
		return s.VerifyAdminPassword(ctx, username, password)
	case "power_user":
		_, ok, err := s.VerifyPowerUserPassword(ctx, username, password)
		return ok, err
	case "user":
		return s.VerifyUserPassword(ctx, username, password)
	default:
		return false, errors.New("role 不合法")
	}
}

func currentSessionIdentity(c *gin.Context) (string, string) {
	return strings.TrimSpace(c.GetString("auth_user")), normalizeAuthAccountRole(c.GetString("auth_role"))
}

func (s *Server) handleAuthTwoFactorStatus(c *gin.Context) {
	username, role := currentSessionIdentity(c)
	if username == "" || role == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	state, err := s.store.GetAccountTwoFactorState(c.Request.Context(), username, role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}

func (s *Server) handleAuthTwoFactorSetupBegin(c *gin.Context) {
	username, role := currentSessionIdentity(c)
	if username == "" || role == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	setup, err := s.store.BeginAccountTwoFactorSetup(c.Request.Context(), username, role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, setup)
}

func (s *Server) handleAuthTwoFactorEnable(c *gin.Context) {
	username, role := currentSessionIdentity(c)
	if username == "" || role == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req authTwoFactorEnableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	state, err := s.store.getAuthAccountTwoFactorState(c.Request.Context(), username, role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pendingSecret := strings.TrimSpace(state.PendingSecret)
	if pendingSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先开始 2FA 绑定"})
		return
	}
	if !verifyTOTPCode(pendingSecret, req.Code, time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "totp_invalid"})
		return
	}
	if err := s.store.EnableAccountTwoFactorFromPending(c.Request.Context(), username, role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	stateResp, err := s.store.GetAccountTwoFactorState(c.Request.Context(), username, role)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "state": stateResp})
}

func (s *Server) handleAuthTwoFactorDisable(c *gin.Context) {
	username, role := currentSessionIdentity(c)
	if username == "" || role == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req authTwoFactorDisableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数不合法"})
		return
	}
	state, err := s.store.getAuthAccountTwoFactorState(c.Request.Context(), username, role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !state.Enabled || strings.TrimSpace(state.Secret) == "" {
		if err := s.store.DisableAccountTwoFactor(c.Request.Context(), username, role); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}
	ok, err := s.store.VerifyPasswordForRole(c.Request.Context(), username, role, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}
	if !verifyTOTPCode(state.Secret, req.Code, time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "totp_invalid"})
		return
	}
	if err := s.store.DisableAccountTwoFactor(c.Request.Context(), username, role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminUserTwoFactorEnable(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	var req adminUserTwoFactorReq
	_ = c.ShouldBindJSON(&req)
	role := normalizeAuthAccountRole(req.Role)
	if username == "" || role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/role 不能为空"})
		return
	}
	setup, err := s.store.ForceEnableAccountTwoFactor(c.Request.Context(), username, role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "setup": setup})
}

func (s *Server) handleAdminUserTwoFactorDisable(c *gin.Context) {
	username := strings.TrimSpace(c.Param("username"))
	var req adminUserTwoFactorReq
	_ = c.ShouldBindJSON(&req)
	role := normalizeAuthAccountRole(req.Role)
	if username == "" || role == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/role 不能为空"})
		return
	}
	if err := s.store.DisableAccountTwoFactor(c.Request.Context(), username, role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
