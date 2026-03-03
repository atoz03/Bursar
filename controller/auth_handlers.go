package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const sessionCookieName = "gpuops_session"

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerReq struct {
	Email                   string `json:"email"`
	Username                string `json:"username"`
	Password                string `json:"password"`
	RealName                string `json:"real_name"`
	StudentID               string `json:"student_id"`
	Advisor                 string `json:"advisor"`
	ExpectedGraduationYear  int    `json:"expected_graduation_year"`
	ExpectedGraduationMonth int    `json:"expected_graduation_month"`
	Phone                   string `json:"phone"`
	AcceptGuideline         bool   `json:"accept_guideline"`
}

type forgotPasswordReq struct {
	Email string `json:"email"`
}

type resetPasswordReq struct {
	Username    string `json:"username"`
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type mailSettingsReq struct {
	SMTPHost   string `json:"smtp_host"`
	SMTPPort   int    `json:"smtp_port"`
	SMTPUser   string `json:"smtp_user"`
	SMTPPass   string `json:"smtp_pass"`
	UpdatePass bool   `json:"update_pass"`
	FromEmail  string `json:"from_email"`
	FromName   string `json:"from_name"`
}

func (s *Server) handleAuthMe(c *gin.Context) {
	if s.cfg.SessionHours == 0 {
		c.JSON(http.StatusOK, gin.H{"authenticated": false, "session_disabled": true})
		return
	}
	secret := strings.TrimSpace(s.cfg.AuthSecret)
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie) == "" {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	p, err := verifySession(secret, cookie, time.Now())
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	role, perms, ok, err := s.store.ResolveSessionRolePerms(c.Request.Context(), p.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"authenticated":       true,
		"username":            p.Username,
		"role":                role,
		"can_view_board":      (perms & permViewBoard) != 0,
		"can_view_nodes":      (perms & permViewNodes) != 0,
		"can_manage_nodes":    (perms & permManageNodes) != 0,
		"can_review_requests": (perms & permReviewRequests) != 0,
		"expires_at":          time.Unix(p.ExpUnix, 0).UTC().Format(time.RFC3339),
		"csrf_token":          p.Nonce,
	})
}

func (s *Server) handleAuthLogin(c *gin.Context) {
	if s.cfg.SessionHours == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_disabled"})
		return
	}

	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/password 不能为空"})
		return
	}

	ok, err := s.store.VerifyAdminPassword(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	role := "admin"
	perms := uint32(permViewBoard | permViewNodes | permManageNodes | permReviewRequests)
	if !ok {
		power, okPower, err := s.store.VerifyPowerUserPassword(c.Request.Context(), req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if okPower {
			role = "power_user"
			perms = 0
			if power.CanViewBoard {
				perms |= permViewBoard
			}
			if power.CanViewNodes {
				perms |= permViewNodes
			}
			if power.CanManageNodes {
				perms |= permManageNodes
			}
			if power.CanReviewRequests {
				perms |= permReviewRequests
			}
		} else {
			hint, hintErr := s.store.LoginHintByUsername(c.Request.Context(), req.Username)
			if hintErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": hintErr.Error()})
				return
			}
			if hint != "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": hint})
				return
			}
			ok, err = s.store.VerifyUserPassword(c.Request.Context(), req.Username, req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
				return
			}
			role = "user"
			perms = 0
		}
	}

	secret := strings.TrimSpace(s.cfg.AuthSecret)
	exp := time.Now().Add(time.Duration(s.cfg.SessionHours) * time.Hour).Unix()
	token, err := signSession(secret, sessionPayload{
		Username: req.Username,
		Role:     role,
		ExpUnix:  exp,
		Nonce:    newNonce(),
		Perms:    perms,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.SetCookie(sessionCookieName, token, int(time.Duration(s.cfg.SessionHours)*time.Hour/time.Second), "/", "", s.cfg.CookieSecure, true)
	c.JSON(http.StatusOK, gin.H{"ok": true, "role": role})
}

func (s *Server) handleAuthLogout(c *gin.Context) {
	// MaxAge=-1 让浏览器删除 cookie
	c.SetCookie(sessionCookieName, "", -1, "/", "", s.cfg.CookieSecure, true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAuthRegister(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	if req.Email == "" || req.Username == "" || req.Password == "" || strings.TrimSpace(req.RealName) == "" ||
		strings.TrimSpace(req.StudentID) == "" || strings.TrimSpace(req.Advisor) == "" || strings.TrimSpace(req.Phone) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请完整填写注册信息"})
		return
	}
	if !req.AcceptGuideline {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先阅读并勾选同意《用户准则》后再提交"})
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil || strings.ContainsAny(req.Email, " \t\r\n") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱格式不合法"})
		return
	}
	if utf8.RuneCountInString(req.Username) > 18 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名不得超过 18 个字符"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码至少 8 位"})
		return
	}
	rawToken, tokenHash, err := newResetToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	expireAt := time.Now().Add(30 * time.Minute)

	ctx := c.Request.Context()
	err = s.store.WithTx(ctx, func(tx *sql.Tx) error {
		return s.store.CreateRegistrationEmailVerificationTx(ctx, tx, UserAccount{
			Username:                req.Username,
			Email:                   req.Email,
			RealName:                req.RealName,
			StudentID:               req.StudentID,
			Advisor:                 req.Advisor,
			ExpectedGraduationYear:  req.ExpectedGraduationYear,
			ExpectedGraduationMonth: req.ExpectedGraduationMonth,
			Phone:                   req.Phone,
		}, req.Password, tokenHash, expireAt)
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settings, err := s.store.GetMailSettings(ctx, s.cfg)
	if err != nil {
		_ = s.store.DeleteRegistrationEmailVerificationByTokenHash(ctx, tokenHash)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	verifyURL := buildRegisterVerifyURL(c, rawToken)
	subject := "GPU Ops 注册邮箱验证"
	body := fmt.Sprintf(
		"你好，\n\n你正在提交 GPU Ops 平台账号注册申请。\n请在 30 分钟内点击以下链接完成邮箱验证，验证后申请才会进入管理员审核：\n%s\n\n若非本人操作，请忽略此邮件。\n\nGPU Ops 团队",
		verifyURL,
	)
	if err := sendPlainTextMail(settings, req.Email, subject, body); err != nil {
		_ = s.store.DeleteRegistrationEmailVerificationByTokenHash(ctx, tokenHash)
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证邮件发送失败，请检查邮箱地址或联系管理员检查 SMTP 配置: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "验证邮件已发送，请前往邮箱点击链接完成提交。验证成功后将进入管理员审核，请勿重复提交。"})
}

func (s *Server) handleAuthRegisterCheck(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	email := strings.TrimSpace(strings.ToLower(c.Query("email")))
	studentID := strings.TrimSpace(c.Query("student_id"))
	localErrs := map[string]string{}
	if utf8.RuneCountInString(username) > 18 {
		localErrs["username"] = "用户名不得超过 18 个字符"
	}
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil || strings.ContainsAny(email, " \t\r\n") {
			localErrs["email"] = "邮箱格式不合法"
		}
	}
	errs, err := s.store.RegistrationAvailability(c.Request.Context(), username, email, studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for k, v := range localErrs {
		errs[k] = v
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "errors": errs})
}

func (s *Server) handleAuthRegisterVerify(c *gin.Context) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("invalid", "验证链接无效，请重新提交注册申请。"))
		return
	}
	tokenHash := sha256Hex(token)
	ctx := c.Request.Context()
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		_, txErr := s.store.ConsumeRegistrationEmailVerificationTx(ctx, tx, tokenHash, time.Now())
		return txErr
	})
	if err != nil {
		switch {
		case errors.Is(err, errRegistrationVerifyTokenInvalid):
			c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("invalid", "验证链接无效，请重新提交注册申请。"))
			return
		case errors.Is(err, errRegistrationVerifyTokenExpired):
			c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("expired", "验证链接已过期，请重新提交注册申请。"))
			return
		case errors.Is(err, errRegistrationVerifyTokenUsed):
			c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("used", "该验证链接已使用，请勿重复点击。"))
			return
		default:
			msg := strings.TrimSpace(err.Error())
			if strings.HasPrefix(msg, "以下信息不可用：") {
				c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("conflict", "验证失败："+msg+"。请返回注册页修改后重试。"))
				return
			}
			c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("error", "验证失败，请稍后重试。"))
			return
		}
	}
	c.Redirect(http.StatusFound, buildRegisterVerifyResultURL("ok", "邮箱验证成功，注册申请已提交，请耐心等待管理员审核。"))
}

func (s *Server) handleAuthForgotPassword(c *gin.Context) {
	var req forgotPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email 不能为空"})
		return
	}

	rawToken, tokenHash, err := newResetToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	expireAt := time.Now().Add(30 * time.Minute)
	username, found, err := s.store.SetPasswordResetTokenByEmail(c.Request.Context(), email, tokenHash, expireAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 为避免枚举账号，无论是否存在都返回 ok。
	if !found {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resetURL := buildResetURL(c, username, rawToken)
	subject := "GPU Ops 密码找回"
	body := fmt.Sprintf("您好，\n\n请在 30 分钟内访问以下链接重置密码：\n%s\n\n若非本人操作，请忽略此邮件。\n\nGPU Ops 团队", resetURL)
	if err := sendResetPasswordMail(settings, email, subject, body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "发送邮件失败，请联系管理员检查 SMTP 配置: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAuthResetPassword(c *gin.Context) {
	var req resetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || strings.TrimSpace(req.Token) == "" || req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数不完整"})
		return
	}
	tokenHash := sha256Hex(strings.TrimSpace(req.Token))
	if err := s.store.ResetPasswordByToken(c.Request.Context(), req.Username, tokenHash, req.NewPassword, time.Now()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAuthChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	username, _ := c.Get("auth_user")
	role, _ := c.Get("auth_role")
	userStr := strings.TrimSpace(fmt.Sprintf("%v", username))
	roleStr := strings.TrimSpace(fmt.Sprintf("%v", role))
	if userStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var err error
	if roleStr == "admin" {
		err = s.store.UpdateAdminPassword(c.Request.Context(), userStr, req.CurrentPassword, req.NewPassword)
	} else if roleStr == "power_user" {
		err = fmt.Errorf("高级用户不允许在网页修改账号信息")
	} else {
		err = s.store.UpdateUserPassword(c.Request.Context(), userStr, req.CurrentPassword, req.NewPassword)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAdminMailSettingsGet(c *gin.Context) {
	settings, err := s.store.GetMailSettings(c.Request.Context(), s.cfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"smtp_host":         settings.SMTPHost,
		"smtp_port":         settings.SMTPPort,
		"smtp_user":         settings.SMTPUser,
		"smtp_password_set": strings.TrimSpace(settings.SMTPPass) != "",
		"from_email":        settings.FromEmail,
		"from_name":         settings.FromName,
	})
}

func (s *Server) handleAdminMailSettingsSet(c *gin.Context) {
	var req mailSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings := MailSettings{
		SMTPHost:  req.SMTPHost,
		SMTPPort:  req.SMTPPort,
		SMTPUser:  req.SMTPUser,
		SMTPPass:  req.SMTPPass,
		FromEmail: req.FromEmail,
		FromName:  req.FromName,
	}
	if err := s.store.UpsertMailSettings(c.Request.Context(), settings, req.UpdatePass); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type bootstrapReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleAdminBootstrap 用于首次创建管理员账号（用于 Web 登录）。
// 规则：仅当 admin_accounts 为空时允许执行，且必须通过 admin_token 调用（避免公开入口）。
func (s *Server) handleAdminBootstrap(c *gin.Context) {
	// 强制要求 Bearer token（禁止用 session 自举，避免逻辑漏洞）
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) || strings.TrimSpace(strings.TrimPrefix(auth, prefix)) != s.cfg.AdminToken {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	count, err := s.store.CountAdminAccounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "already_bootstrapped"})
		return
	}

	var req bootstrapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username/password 不能为空"})
		return
	}

	if err := s.store.CreateAdminAccount(c.Request.Context(), req.Username, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func newResetToken() (string, string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b[:])
	return token, sha256Hex(token), nil
}

func sha256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func buildResetURL(c *gin.Context, username string, token string) string {
	scheme := "http"
	if strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https") || c.Request.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/reset-password?username=%s&token=%s",
		scheme,
		c.Request.Host,
		url.QueryEscape(username),
		url.QueryEscape(token),
	)
}

func buildRegisterVerifyURL(c *gin.Context, token string) string {
	scheme := "http"
	if strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")), "https") || c.Request.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/auth/register/verify?token=%s", scheme, c.Request.Host, url.QueryEscape(token))
}

func buildRegisterVerifyResultURL(status string, message string) string {
	q := url.Values{}
	if strings.TrimSpace(status) != "" {
		q.Set("email_verify", status)
	}
	if strings.TrimSpace(message) != "" {
		q.Set("verify_msg", message)
	}
	enc := q.Encode()
	if enc == "" {
		return "/register"
	}
	return "/register?" + enc
}
