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
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const sessionCookieName = "gpuops_session"

type loginReq struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	TOTPCode      string `json:"totp_code"`
	CaptchaToken  string `json:"captcha_token"`
	CaptchaID     string `json:"captcha_id"`
	CaptchaOption int    `json:"captcha_option"`
}

type registerReq struct {
	Email                   string `json:"email"`
	Username                string `json:"username"`
	Password                string `json:"password"`
	CaptchaToken            string `json:"captcha_token"`
	CaptchaID               string `json:"captcha_id"`
	CaptchaOption           int    `json:"captcha_option"`
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
	writeWithServerTime := func(status int, body gin.H) {
		now := nowInBeijing()
		if settings, err := s.store.GetPlatformSettings(c.Request.Context()); err == nil {
			body["platform_name"] = settings.PlatformName
			body["registration_allowed_email_domains"] = settings.RegistrationAllowedEmailDomains
			body["provision_ssh_host"] = settings.ProvisionSSHHost
			body["setup_completed"] = settings.SetupCompleted
		}
		body["server_now"] = formatRFC3339InBeijing(now)
		body["server_tz_name"] = beijingTZName
		body["server_tz_offset_minutes"] = beijingOffsetMinutes(now)
		c.JSON(status, body)
	}

	if s.cfg.SessionHours == 0 {
		writeWithServerTime(http.StatusOK, gin.H{"authenticated": false, "session_disabled": true})
		return
	}
	secret := strings.TrimSpace(s.cfg.AuthSecret)
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie) == "" {
		writeWithServerTime(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	p, err := verifySession(secret, cookie, time.Now())
	if err != nil {
		writeWithServerTime(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	role, perms, ok, err := s.store.ResolveSessionRolePerms(c.Request.Context(), p.Username)
	if err != nil {
		writeWithServerTime(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		writeWithServerTime(http.StatusOK, gin.H{"authenticated": false})
		return
	}
	twoFactorState, err := s.store.GetAccountTwoFactorState(c.Request.Context(), p.Username, role)
	if err != nil {
		writeWithServerTime(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	writeWithServerTime(http.StatusOK, gin.H{
		"authenticated":             true,
		"username":                  p.Username,
		"role":                      role,
		"two_factor_enabled":        twoFactorState.Enabled,
		"can_view_board":            (perms & permViewBoard) != 0,
		"can_view_nodes":            (perms & permViewNodes) != 0,
		"can_manage_nodes":          (perms & permManageNodes) != 0,
		"can_manage_points":         (perms & permAnyPoints) != 0,
		"can_points_users":          (perms & permPointsUsers) != 0,
		"can_points_batch_filtered": (perms & permPointsBatchFiltered) != 0,
		"can_points_batch_all":      (perms & permPointsBatchAll) != 0,
		"can_points_records":        (perms & permPointsRecords) != 0,
		"can_points_monthly":        (perms & permPointsMonthly) != 0,
		"can_points_special_rules":  (perms & permPointsSpecialRules) != 0,
		"can_review_requests":       (perms & permReviewRequests) != 0,
		"can_manage_platform_users": (perms & permManagePlatformUsers) != 0,
		"expires_at":                formatRFC3339InBeijing(time.Unix(p.ExpUnix, 0)),
		"csrf_token":                p.Nonce,
	})
}

func registerRetryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	sec := int(d / time.Second)
	if d%time.Second != 0 {
		sec++
	}
	if sec < 1 {
		sec = 1
	}
	return sec
}

func writeRegisterRateLimited(c *gin.Context, errMsg string, retryAt time.Time, now time.Time) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":               errMsg,
		"retry_after_seconds": registerRetryAfterSeconds(retryAt.Sub(now)),
		"retry_at":            formatRFC3339InBeijing(retryAt),
	})
}

func writeLoginCaptchaError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errAuthCaptchaUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "captcha_unavailable"})
	case errors.Is(err, errRegisterCaptchaExpired):
		c.JSON(http.StatusBadRequest, gin.H{"error": "captcha_expired"})
	case errors.Is(err, errRegisterCaptchaUsed):
		c.JSON(http.StatusBadRequest, gin.H{"error": "captcha_used"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "captcha_invalid"})
	}
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
	now := time.Now()
	if s.turnstileEnabled() {
		if strings.TrimSpace(req.CaptchaToken) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "captcha_missing"})
			return
		}
		if err := s.verifyTurnstile(c.Request.Context(), req.CaptchaToken, "login"); err != nil {
			writeLoginCaptchaError(c, err)
			return
		}
	} else {
		if strings.TrimSpace(req.CaptchaID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "captcha_missing"})
			return
		}
		if err := s.store.VerifyRegisterCaptcha(c.Request.Context(), req.CaptchaID, req.CaptchaOption, now); err != nil {
			writeLoginCaptchaError(c, err)
			return
		}
	}

	ok, err := s.store.VerifyAdminPassword(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	role := "admin"
	perms := uint32(permViewBoard | permViewNodes | permManageNodes | permReviewRequests | permManagePoints | permManagePlatformUsers | permPointsUsers | permPointsBatchFiltered | permPointsBatchAll | permPointsRecords | permPointsMonthly | permPointsSpecialRules)
	var twoFactorState authAccountTwoFactorState
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
			if power.CanManagePoints || power.CanPointsUsers || power.CanPointsBatchFiltered || power.CanPointsBatchAll || power.CanPointsRecords || power.CanPointsMonthly || power.CanPointsSpecialRules {
				perms |= permManagePoints
			}
			if power.CanPointsUsers {
				perms |= permPointsUsers
			}
			if power.CanPointsBatchFiltered {
				perms |= permPointsBatchFiltered
			}
			if power.CanPointsBatchAll {
				perms |= permPointsBatchAll
			}
			if power.CanPointsRecords {
				perms |= permPointsRecords
			}
			if power.CanPointsMonthly {
				perms |= permPointsMonthly
			}
			if power.CanPointsSpecialRules {
				perms |= permPointsSpecialRules
			}
			if power.CanReviewRequests {
				perms |= permReviewRequests
			}
			if power.CanManagePlatformUsers {
				perms |= permManagePlatformUsers
			}
			twoFactorState, err = s.store.getAuthAccountTwoFactorState(c.Request.Context(), req.Username, role)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			hint, hintErr := s.store.LoginHintByUsername(c.Request.Context(), req.Username)
			if hintErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": hintErr.Error()})
				return
			}
			if hint != "" {
				if err := s.consumeLocalLoginCaptcha(c.Request.Context(), req.CaptchaID, now); err != nil {
					writeLoginCaptchaError(c, err)
					return
				}
				c.JSON(http.StatusUnauthorized, gin.H{"error": hint})
				return
			}
			ok, err = s.store.VerifyUserPassword(c.Request.Context(), req.Username, req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if !ok {
				if err := s.consumeLocalLoginCaptcha(c.Request.Context(), req.CaptchaID, now); err != nil {
					writeLoginCaptchaError(c, err)
					return
				}
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
				return
			}
			role = "user"
			perms = 0
			twoFactorState, err = s.store.getAuthAccountTwoFactorState(c.Request.Context(), req.Username, role)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	} else {
		twoFactorState, err = s.store.getAuthAccountTwoFactorState(c.Request.Context(), req.Username, role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if twoFactorState.Enabled {
		if strings.TrimSpace(req.TOTPCode) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "totp_required"})
			return
		}
		if strings.TrimSpace(twoFactorState.Secret) == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "账号 2FA 配置异常，请联系管理员"})
			return
		}
		if !verifyTOTPCode(twoFactorState.Secret, req.TOTPCode, now) {
			if err := s.consumeLocalLoginCaptcha(c.Request.Context(), req.CaptchaID, now); err != nil {
				writeLoginCaptchaError(c, err)
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "totp_invalid"})
			return
		}
	}
	if err := s.consumeLocalLoginCaptcha(c.Request.Context(), req.CaptchaID, now); err != nil {
		writeLoginCaptchaError(c, err)
		return
	}

	if err := s.store.RecordSuccessfulLogin(c.Request.Context(), req.Username, role, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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

	// SameSite=Lax：与 X-CSRF-Token 校验形成双重防护，避免跨站携带会话 cookie。
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, token, int(time.Duration(s.cfg.SessionHours)*time.Hour/time.Second), "/", "", s.cfg.CookieSecure, true)
	c.JSON(http.StatusOK, gin.H{"ok": true, "role": role})
}

func (s *Server) handleAuthLogout(c *gin.Context) {
	// MaxAge=-1 让浏览器删除 cookie
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", s.cfg.CookieSecure, true)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleAuthRegister(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.Username = strings.TrimSpace(req.Username)
	req.StudentID = normalizeStudentID(req.StudentID)
	clientIP := trimmedClientIP(c.ClientIP())
	userAgent := trimmedUserAgent(c.GetHeader("User-Agent"))
	recordEvent := func(decision string, reason string) {
		_ = s.store.InsertRegistrationSecurityEvent(c.Request.Context(), RegistrationSecurityEvent{
			Action:    "register_submit",
			Decision:  decision,
			Reason:    reason,
			ClientIP:  clientIP,
			Username:  req.Username,
			Email:     req.Email,
			StudentID: req.StudentID,
			UserAgent: userAgent,
		})
	}
	platformSettings, err := s.store.GetPlatformSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !platformSettings.SetupCompleted {
		recordEvent("deny", "setup_required")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "setup_required"})
		return
	}

	if req.Email == "" || req.Username == "" || req.Password == "" || strings.TrimSpace(req.RealName) == "" ||
		req.StudentID == "" || strings.TrimSpace(req.Advisor) == "" || strings.TrimSpace(req.Phone) == "" {
		recordEvent("deny", "missing_required_fields")
		c.JSON(http.StatusBadRequest, gin.H{"error": "请完整填写注册信息"})
		return
	}
	if !req.AcceptGuideline {
		recordEvent("deny", "guideline_not_accepted")
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先阅读并勾选同意《用户准则》后再提交"})
		return
	}
	normalizedEmail, emailLocal, emailDomain, err := normalizeRegisterEmail(req.Email, platformSettings.RegistrationAllowedEmailDomains)
	if err != nil {
		recordEvent("deny", "invalid_register_email_or_student")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Email = normalizedEmail
	if err := validateRegisterUsername(req.Username, emailLocal); err != nil {
		recordEvent("deny", "invalid_username_rule")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateStrongPassword(req.Password); err != nil {
		recordEvent("deny", "weak_password")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	stats, err := s.store.LoadRegistrationRateStats(
		c.Request.Context(),
		clientIP,
		req.Email,
		now.Add(-registerIPWindow),
		now.Add(-registerEmailWindow),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if stats.IPCount >= registerIPLimit {
		recordEvent("deny", "rate_limited_by_ip_window")
		retryAt := now.Add(registerIPWindow)
		if stats.IPWindowRetryAt != nil && stats.IPWindowRetryAt.After(now) {
			retryAt = *stats.IPWindowRetryAt
		}
		writeRegisterRateLimited(c, "请求过于频繁，请稍后再试", retryAt, now)
		return
	}
	if stats.EmailCount >= registerEmailLimit {
		recordEvent("deny", "rate_limited_by_email_window")
		retryAt := now.Add(registerEmailWindow)
		if stats.EmailWindowRetryAt != nil && stats.EmailWindowRetryAt.After(now) {
			retryAt = *stats.EmailWindowRetryAt
		}
		writeRegisterRateLimited(c, "该邮箱请求过于频繁，请稍后再试", retryAt, now)
		return
	}
	if stats.LastEmailAt != nil && now.Sub(*stats.LastEmailAt) < registerEmailCooldown {
		recordEvent("deny", "cooldown_by_email")
		retryAt := stats.LastEmailAt.Add(registerEmailCooldown)
		if !retryAt.After(now) {
			retryAt = now.Add(time.Second)
		}
		writeRegisterRateLimited(c, "该邮箱请求过于频繁，请稍后再试", retryAt, now)
		return
	}
	blocked, err := s.store.IsDisposableEmailDomainBlocked(c.Request.Context(), emailDomain)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if blocked {
		recordEvent("deny", "blocked_disposable_domain")
		c.JSON(http.StatusBadRequest, gin.H{"error": "该邮箱域名不允许注册，请使用学校正式邮箱"})
		return
	}
	var captchaErr error
	if s.turnstileEnabled() {
		if strings.TrimSpace(req.CaptchaToken) == "" {
			recordEvent("deny", "captcha_missing")
			c.JSON(http.StatusBadRequest, gin.H{"error": "请先完成人机验证"})
			return
		}
		captchaErr = s.verifyTurnstile(c.Request.Context(), req.CaptchaToken, "register")
	} else {
		if strings.TrimSpace(req.CaptchaID) == "" {
			recordEvent("deny", "captcha_missing")
			c.JSON(http.StatusBadRequest, gin.H{"error": "验证码已失效，请刷新后重试"})
			return
		}
		captchaErr = s.store.VerifyAndConsumeRegisterCaptcha(c.Request.Context(), req.CaptchaID, req.CaptchaOption, now)
	}
	if captchaErr != nil {
		switch {
		case errors.Is(captchaErr, errAuthCaptchaUnavailable):
			recordEvent("deny", "captcha_unavailable")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "人机验证服务暂时不可用，请稍后重试"})
		case errors.Is(captchaErr, errRegisterCaptchaExpired):
			recordEvent("deny", "captcha_expired")
			c.JSON(http.StatusBadRequest, gin.H{"error": "验证码已过期，请刷新后重试"})
		case errors.Is(captchaErr, errRegisterCaptchaUsed):
			recordEvent("deny", "captcha_used")
			c.JSON(http.StatusBadRequest, gin.H{"error": "验证码已失效，请刷新后重试"})
		default:
			recordEvent("deny", "captcha_invalid")
			if s.turnstileEnabled() {
				c.JSON(http.StatusBadRequest, gin.H{"error": "人机验证未通过，请重试"})
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "验证码错误，请重试"})
			}
		}
		return
	}

	rawToken, tokenHash, err := newResetToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	expireAt := now.Add(30 * time.Minute)

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
		recordEvent("deny", "create_registration_verification_failed")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settings, err := s.store.GetMailSettings(ctx, s.cfg)
	if err != nil {
		_ = s.store.DeleteRegistrationEmailVerificationByTokenHash(ctx, tokenHash)
		recordEvent("deny", "load_mail_settings_failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	verifyURL := buildRegisterVerifyURL(c, rawToken)
	platformName := s.platformName(ctx)
	subject := platformName + " 注册邮箱验证"
	body := fmt.Sprintf(
		"你好，\n\n你正在提交 %s 平台账号注册申请。\n请在 30 分钟内点击以下链接完成邮箱验证，验证后申请才会进入管理员审核：\n%s\n\n若非本人操作，请忽略此邮件。\n\n%s 团队",
		platformName,
		verifyURL,
		platformName,
	)
	if err := sendPlainTextMail(settings, req.Email, subject, body); err != nil {
		_ = s.store.DeleteRegistrationEmailVerificationByTokenHash(ctx, tokenHash)
		recordEvent("deny", "send_verification_mail_failed")
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证邮件发送失败，请检查邮箱地址或联系管理员检查 SMTP 配置: " + err.Error()})
		return
	}
	recordEvent("allow", "verification_mail_sent")

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "验证邮件已发送，请前往邮箱点击链接完成提交。验证成功后将进入管理员审核，请勿重复提交。"})
}

func (s *Server) handleAuthRegisterCheck(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	email := strings.TrimSpace(c.Query("email"))
	studentID := normalizeStudentID(c.Query("student_id"))
	localErrs := map[string]string{}
	emailLocal := ""
	platformSettings, err := s.store.GetPlatformSettings(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if email != "" {
		normalizedEmail, local, domain, err := normalizeRegisterEmail(email, platformSettings.RegistrationAllowedEmailDomains)
		if err != nil {
			localErrs["email"] = err.Error()
		} else {
			email = normalizedEmail
			emailLocal = local
			blocked, blockErr := s.store.IsDisposableEmailDomainBlocked(c.Request.Context(), domain)
			if blockErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": blockErr.Error()})
				return
			}
			if blocked {
				localErrs["email"] = "该邮箱域名不允许注册，请使用可信邮箱"
			}
		}
	}
	if username != "" {
		if err := validateRegisterUsername(username, emailLocal); err != nil {
			localErrs["username"] = err.Error()
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
	platformName := s.platformName(c.Request.Context())
	subject := platformName + " 密码找回"
	body := fmt.Sprintf("您好，\n\n请在 30 分钟内访问以下链接重置密码：\n%s\n\n若非本人操作，请忽略此邮件。\n\n%s 团队", resetURL, platformName)
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
	if err := validateStrongPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	if err := validateStrongPassword(req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	if !strings.HasPrefix(auth, prefix) || !constantTimeTokenEqual(strings.TrimSpace(strings.TrimPrefix(auth, prefix)), s.cfg.AdminToken) {
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
	if err := validateStrongPassword(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
